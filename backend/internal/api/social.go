package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type Post struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Author    string    `json:"author"`
	AvatarURL string    `json:"avatar_url"`
	Content   string    `json:"content"`
	ImageURL  string    `json:"image_url"`
	Township  string    `json:"township"`
	CreatedAt time.Time `json:"created_at"`
	Reactions int64     `json:"reactions"`
	Comments  int64     `json:"comments"`
}

const postFields = `p.id, p.user_id, u.display_name, COALESCE(pr.avatar_url,''),
 p.content, p.image_url, p.township, p.created_at,
 (SELECT count(*) FROM social.reactions r WHERE r.post_id=p.id),
 (SELECT count(*) FROM social.comments c WHERE c.post_id=p.id)`

const postFrom = ` FROM social.posts p
 JOIN social.users u ON u.id = p.user_id
 LEFT JOIN social.user_profiles pr ON pr.user_id = p.user_id `

func scanPost(row pgx.Row) (*Post, error) {
	var p Post
	err := row.Scan(&p.ID, &p.UserID, &p.Author, &p.AvatarURL, &p.Content,
		&p.ImageURL, &p.Township, &p.CreatedAt, &p.Reactions, &p.Comments)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// handleCreatePost creates a feed post in the social schema only.
func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "authentication required")
		return
	}
	var req struct {
		Content  string `json:"content"`
		ImageURL string `json:"image_url"`
		Township string `json:"township"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	req.Content = trimSpace(req.Content)
	if len(req.Content) == 0 {
		errorJSON(w, http.StatusBadRequest, "content is required")
		return
	}
	if len(req.Content) > 2000 {
		errorJSON(w, http.StatusBadRequest, "content must be at most 2000 characters")
		return
	}
	req.ImageURL = trimLimit(req.ImageURL, 500)
	req.Township = trimLimit(req.Township, 80)
	var id int64
	if err := s.Store.Social.QueryRow(r.Context(),
		`INSERT INTO social.posts (user_id, content, image_url, township)
		 VALUES ($1,$2,$3,$4) RETURNING id`,
		uid, req.Content, req.ImageURL, req.Township).Scan(&id); err != nil {
		serverError(w, "create post", err)
		return
	}
	p, err := scanPost(s.Store.Social.QueryRow(r.Context(), `SELECT `+postFields+postFrom+`WHERE p.id=$1`, id))
	if err != nil {
		serverError(w, "read post", err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

// handleListPosts returns the township-filtered feed, newest first.
func (s *Server) handleListPosts(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() { s.Metrics.ObserveFeed(time.Since(start)) }()
	q := r.URL.Query()
	township := trimLimit(q.Get("township"), 80)
	limit := 20
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = v
	}
	cursor := int64(0)
	if v, err := strconv.ParseInt(q.Get("cursor"), 10, 64); err == nil {
		cursor = v
	}
	rows, err := s.Store.Social.Query(r.Context(),
		`SELECT `+postFields+postFrom+
			`WHERE ($1::text='' OR p.township=$1) AND ($2::bigint=0 OR p.id < $2)
			 ORDER BY p.id DESC LIMIT $3`,
		township, cursor, limit)
	if err != nil {
		serverError(w, "list posts", err)
		return
	}
	defer rows.Close()
	posts := []Post{}
	var last int64
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			serverError(w, "scan post", err)
			return
		}
		posts = append(posts, *p)
		last = p.ID
	}
	if err := rows.Err(); err != nil {
		serverError(w, "iterate posts", err)
		return
	}
	next := int64(0)
	if len(posts) == limit {
		next = last
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": posts, "next_cursor": next})
}

// handleGetPost fetches one post.
func (s *Server) handleGetPost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	p, err := scanPost(s.Store.Social.QueryRow(r.Context(),
		`SELECT `+postFields+postFrom+`WHERE p.id=$1`, id))
	if err == pgx.ErrNoRows {
		errorJSON(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		serverError(w, "read post", err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// handleReact toggles the caller's reaction on a post.
func (s *Server) handleReact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	uid, _ := userID(r)
	var req struct {
		Kind string `json:"kind"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	if req.Kind == "" {
		req.Kind = "like"
	}
	if req.Kind != "like" && req.Kind != "love" {
		errorJSON(w, http.StatusBadRequest, "kind must be like or love")
		return
	}
	var exists bool
	if err := s.Store.Social.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM social.posts WHERE id=$1)`, id).Scan(&exists); err != nil {
		serverError(w, "check post", err)
		return
	}
	if !exists {
		errorJSON(w, http.StatusNotFound, "post not found")
		return
	}
	var current string
	err := s.Store.Social.QueryRow(r.Context(),
		`SELECT kind FROM social.reactions WHERE post_id=$1 AND user_id=$2`, id, uid).Scan(&current)
	liked := true
	switch {
	case err == nil && current == req.Kind:
		if _, err := s.Store.Social.Exec(r.Context(),
			`DELETE FROM social.reactions WHERE post_id=$1 AND user_id=$2`, id, uid); err != nil {
			serverError(w, "remove reaction", err)
			return
		}
		liked = false
	case err == nil:
		if _, err := s.Store.Social.Exec(r.Context(),
			`UPDATE social.reactions SET kind=$3 WHERE post_id=$1 AND user_id=$2`, id, uid, req.Kind); err != nil {
			serverError(w, "update reaction", err)
			return
		}
	default:
		if _, err := s.Store.Social.Exec(r.Context(),
			`INSERT INTO social.reactions (post_id, user_id, kind) VALUES ($1,$2,$3)`, id, uid, req.Kind); err != nil {
			serverError(w, "add reaction", err)
			return
		}
	}
	var count int64
	if err := s.Store.Social.QueryRow(r.Context(),
		`SELECT count(*) FROM social.reactions WHERE post_id=$1`, id).Scan(&count); err != nil {
		serverError(w, "count reactions", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"liked": liked, "reactions": count})
}

// handleAddComment appends a comment to a post.
func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
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
	if len(req.Body) == 0 || len(req.Body) > 500 {
		errorJSON(w, http.StatusBadRequest, "body must be 1-500 characters")
		return
	}
	var exists bool
	if err := s.Store.Social.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM social.posts WHERE id=$1)`, id).Scan(&exists); err != nil {
		serverError(w, "check post", err)
		return
	}
	if !exists {
		errorJSON(w, http.StatusNotFound, "post not found")
		return
	}
	var commentID int64
	var createdAt time.Time
	if err := s.Store.Social.QueryRow(r.Context(),
		`INSERT INTO social.comments (post_id, user_id, body) VALUES ($1,$2,$3)
		 RETURNING id, created_at`, id, uid, req.Body).Scan(&commentID, &createdAt); err != nil {
		serverError(w, "add comment", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": commentID, "post_id": id, "user_id": uid, "body": req.Body, "created_at": createdAt,
	})
}

// handleListComments returns a post's comments oldest-first.
func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	rows, err := s.Store.Social.Query(r.Context(),
		`SELECT c.id, c.user_id, u.display_name, c.body, c.created_at
		 FROM social.comments c JOIN social.users u ON u.id=c.user_id
		 WHERE c.post_id=$1 ORDER BY c.id ASC LIMIT 100`, id)
	if err != nil {
		serverError(w, "list comments", err)
		return
	}
	defer rows.Close()
	type comment struct {
		ID        int64     `json:"id"`
		UserID    int64     `json:"user_id"`
		Author    string    `json:"author"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
	}
	comments := []comment{}
	for rows.Next() {
		var c comment
		if err := rows.Scan(&c.ID, &c.UserID, &c.Author, &c.Body, &c.CreatedAt); err != nil {
			serverError(w, "scan comment", err)
			return
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		serverError(w, "iterate comments", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
}
