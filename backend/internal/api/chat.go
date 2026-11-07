package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type Thread struct {
	ID           int64     `json:"id"`
	Mode         string    `json:"mode"`
	OwnerID      int64     `json:"owner_id"`
	PeerID       int64     `json:"peer_id"`
	ProductID    int64     `json:"product_id"`
	ProductTitle string    `json:"product_title"`
	CreatedAt    time.Time `json:"created_at"`
}

const threadFields = `t.id, t.mode, t.owner_id, COALESCE(t.peer_id,0),
 COALESCE(t.product_id,0), COALESCE(p.title,''), t.created_at`

const threadFrom = ` FROM social.chat_threads t
 LEFT JOIN market.products p ON p.id = t.product_id `

func scanThread(row pgx.Row) (*Thread, error) {
	var t Thread
	err := row.Scan(&t.ID, &t.Mode, &t.OwnerID, &t.PeerID, &t.ProductID, &t.ProductTitle, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// handleCreateThread opens a personal or business chat thread. Business
// threads are anchored to a market product and the peer is its seller.
func (s *Server) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var req struct {
		Mode      string `json:"mode"`
		PeerID    int64  `json:"peer_id"`
		ProductID int64  `json:"product_id"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	var peerID, productID int64
	switch req.Mode {
	case "personal":
		if req.PeerID <= 0 {
			errorJSON(w, http.StatusBadRequest, "peer_id is required for personal threads")
			return
		}
		if req.PeerID == uid {
			errorJSON(w, http.StatusBadRequest, "cannot open a thread with yourself")
			return
		}
		var exists bool
		if err := s.Store.Social.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM social.users WHERE id=$1)`, req.PeerID).Scan(&exists); err != nil {
			serverError(w, "check peer", err)
			return
		}
		if !exists {
			errorJSON(w, http.StatusNotFound, "peer not found")
			return
		}
		peerID = req.PeerID
	case "business":
		if req.ProductID <= 0 {
			errorJSON(w, http.StatusBadRequest, "product_id is required for business threads")
			return
		}
		var sellerID int64
		var active bool
		err := s.Store.Social.QueryRow(r.Context(),
			`SELECT seller_id, active FROM market.products WHERE id=$1`,
			req.ProductID).Scan(&sellerID, &active)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errorJSON(w, http.StatusNotFound, "product not found")
			} else {
				serverError(w, "load product", err)
			}
			return
		}
		if !active {
			errorJSON(w, http.StatusConflict, "product no longer available")
			return
		}
		peerID = sellerID
		productID = req.ProductID
	default:
		errorJSON(w, http.StatusBadRequest, "mode must be personal or business")
		return
	}
	var id int64
	var createdAt time.Time
	if err := s.Store.Social.QueryRow(r.Context(),
		`INSERT INTO social.chat_threads (mode, owner_id, peer_id, product_id)
		 VALUES ($1,$2,$3,NULLIF($4,0)) RETURNING id, created_at`,
		req.Mode, uid, peerID, productID).Scan(&id, &createdAt); err != nil {
		serverError(w, "create thread", err)
		return
	}
	writeJSON(w, http.StatusCreated, Thread{
		ID: id, Mode: req.Mode, OwnerID: uid, PeerID: peerID,
		ProductID: productID, CreatedAt: createdAt,
	})
}

// handleListThreads returns the caller's threads (owner or peer side).
func (s *Server) handleListThreads(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	rows, err := s.Store.Social.Query(r.Context(),
		`SELECT `+threadFields+threadFrom+` WHERE t.owner_id=$1 OR t.peer_id=$1
		 ORDER BY t.id DESC LIMIT 50`, uid)
	if err != nil {
		serverError(w, "list threads", err)
		return
	}
	defer rows.Close()
	threads := []Thread{}
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			serverError(w, "scan thread", err)
			return
		}
		threads = append(threads, *t)
	}
	if err := rows.Err(); err != nil {
		serverError(w, "iterate threads", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"threads": threads})
}

// threadAuth loads a thread and enforces membership.
func (s *Server) threadAuth(w http.ResponseWriter, r *http.Request) (*Thread, bool) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return nil, false
	}
	uid, _ := userID(r)
	t, err := scanThread(s.Store.Social.QueryRow(r.Context(),
		`SELECT `+threadFields+threadFrom+`WHERE t.id=$1`, id))
	if err == pgx.ErrNoRows {
		errorJSON(w, http.StatusNotFound, "thread not found")
		return nil, false
	}
	if err != nil {
		serverError(w, "load thread", err)
		return nil, false
	}
	if t.OwnerID != uid && t.PeerID != uid {
		errorJSON(w, http.StatusForbidden, "you are not a member of this thread")
		return nil, false
	}
	return t, true
}

// handlePostMessage appends a message to a thread the caller belongs to.
func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadAuth(w, r)
	if !ok {
		return
	}
	uid, _ := userID(r)
	var req struct {
		Body string `json:"body"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	req.Body = trimSpace(req.Body)
	if len(req.Body) == 0 || len(req.Body) > 2000 {
		errorJSON(w, http.StatusBadRequest, "body must be 1-2000 characters")
		return
	}
	var msgID int64
	var createdAt time.Time
	if err := s.Store.Social.QueryRow(r.Context(),
		`INSERT INTO social.chat_messages (thread_id, sender_id, body) VALUES ($1,$2,$3)
		 RETURNING id, created_at`, t.ID, uid, req.Body).Scan(&msgID, &createdAt); err != nil {
		serverError(w, "post message", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": msgID, "thread_id": t.ID, "sender_id": uid,
		"body": req.Body, "created_at": createdAt,
	})
}

// handleListMessages returns a thread's messages oldest-first.
func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	t, ok := s.threadAuth(w, r)
	if !ok {
		return
	}
	rows, err := s.Store.Social.Query(r.Context(),
		`SELECT m.id, m.sender_id, m.body, m.created_at
		 FROM social.chat_messages m WHERE m.thread_id=$1 ORDER BY m.id ASC LIMIT 200`, t.ID)
	if err != nil {
		serverError(w, "list messages", err)
		return
	}
	defer rows.Close()
	type message struct {
		ID        int64     `json:"id"`
		SenderID  int64     `json:"sender_id"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
	}
	messages := []message{}
	for rows.Next() {
		var m message
		if err := rows.Scan(&m.ID, &m.SenderID, &m.Body, &m.CreatedAt); err != nil {
			serverError(w, "scan message", err)
			return
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		serverError(w, "iterate messages", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": t, "messages": messages})
}
