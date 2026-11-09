package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"nexus/backend/internal/store"
)

type datingProfile struct {
	UserID   int64  `json:"user_id"`
	Nickname string `json:"nickname"`
	Bio      string `json:"bio"`
	Avatar   string `json:"avatar_url"`
	Hidden   bool   `json:"is_hidden_from_friends"`
}

// nodeID wraps a dating user id in an opaque, URL-safe token so raw table
// ids are never exposed to clients.
func nodeIDEncode(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte("d" + strconv.FormatInt(id, 10)))
}

func nodeIDDecode(s string) (int64, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(raw) < 2 || raw[0] != 'd' {
		return 0, false
	}
	id, err := strconv.ParseInt(string(raw[1:]), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// anonymousHandle derives a stable pseudonym (Bee-XXXX) from the user id.
func anonymousHandle(id int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("nexus-radar:%d", id)))
	return "Bee-" + hex.EncodeToString(sum[:2])
}

// distanceBucket buckets a distance in meters into the coarse labels the
// prototype UI shows; exact distances are never sent to clients.
func distanceBucket(m float64) string {
	switch {
	case m < 500:
		return "500m"
	case m < 1000:
		return "1km"
	default:
		return "2km"
	}
}

func (s *Server) handleGetDatingProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdult(w, r) {
		return
	}
	uid, _ := userID(r)
	var dp datingProfile
	err := s.Store.DatingConn(r.Context(), func(q store.Querier) error {
		return q.QueryRow(r.Context(),
			`SELECT user_id, nickname, bio, avatar_url, is_hidden_from_friends
			 FROM dating.dating_profiles WHERE user_id=$1`, uid).
			Scan(&dp.UserID, &dp.Nickname, &dp.Bio, &dp.Avatar, &dp.Hidden)
	})
	if err == pgx.ErrNoRows {
		errorJSON(w, http.StatusNotFound, "dating profile not created yet")
		return
	}
	if err != nil {
		serverError(w, "read dating profile", err)
		return
	}
	writeJSON(w, http.StatusOK, dp)
}

// handlePutDatingProfile upserts the caller's row in the isolated schema.
func (s *Server) handlePutDatingProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdult(w, r) {
		return
	}
	uid, _ := userID(r)
	var req struct {
		Nickname string `json:"nickname"`
		Bio      string `json:"bio"`
		Avatar   string `json:"avatar_url"`
		Hidden   bool   `json:"is_hidden_from_friends"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	req.Nickname = trimSpace(req.Nickname)
	if len(req.Nickname) < 2 || len(req.Nickname) > 60 {
		errorJSON(w, http.StatusBadRequest, "nickname must be 2-60 characters")
		return
	}
	req.Bio = trimSpace(req.Bio)
	if len(req.Bio) > 500 {
		errorJSON(w, http.StatusBadRequest, "bio must be at most 500 characters")
		return
	}
	req.Avatar = trimLimit(req.Avatar, 500)
	err := s.Store.DatingConn(r.Context(), func(q store.Querier) error {
		_, err := q.Exec(r.Context(),
			`INSERT INTO dating.dating_profiles (user_id, nickname, bio, avatar_url, is_hidden_from_friends, updated_at)
			 VALUES ($1,$2,$3,$4,$5, now())
			 ON CONFLICT (user_id) DO UPDATE
			 SET nickname=EXCLUDED.nickname, bio=EXCLUDED.bio,
			     avatar_url=EXCLUDED.avatar_url, is_hidden_from_friends=EXCLUDED.is_hidden_from_friends,
			     updated_at=now()`,
			uid, req.Nickname, req.Bio, req.Avatar, req.Hidden)
		return err
	})
	if err != nil {
		serverError(w, "save dating profile", err)
		return
	}
	writeJSON(w, http.StatusOK, datingProfile{
		UserID: uid, Nickname: req.Nickname, Bio: req.Bio, Avatar: req.Avatar, Hidden: req.Hidden,
	})
}

type radarNode struct {
	NodeID   string `json:"node_id"`
	Handle   string `json:"handle"`
	Avatar   string `json:"avatar_url"`
	Distance string `json:"distance_bucket"`
}

// handleRadar returns k-anonymous nearby dating users. Exact coordinates,
// real names and user ids are never exposed; fewer than k=3 candidates
// degrades to an empty result to prevent re-identification.
func (s *Server) handleRadar(w http.ResponseWriter, r *http.Request) {
	pushStart := time.Now()
	defer func() { s.Metrics.ObserveRadarPush(time.Since(pushStart)) }()
	if !s.requireAdult(w, r) {
		return
	}
	uid, _ := userID(r)
	q := r.URL.Query()
	lat, err1 := strconv.ParseFloat(q.Get("lat"), 64)
	lng, err2 := strconv.ParseFloat(q.Get("lng"), 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		errorJSON(w, http.StatusBadRequest, "lat and lng are required and must be valid coordinates")
		return
	}
	radius := 2000.0
	if v, err := strconv.ParseFloat(q.Get("radius"), 64); err == nil && v >= 100 && v <= 5000 {
		radius = v
	}

	var nodes []radarNode
	degraded := false
	err := s.Store.DatingConn(r.Context(), func(qq store.Querier) error {
		ctx := r.Context()
		if _, err := qq.Exec(ctx,
			`INSERT INTO dating.radar_presence (user_id, geog, updated_at)
			 VALUES ($1, ST_SetSRID(ST_MakePoint($2,$3),4326)::geography, now())
			 ON CONFLICT (user_id) DO UPDATE
			 SET geog=EXCLUDED.geog, updated_at=now()`,
			uid, lng, lat); err != nil {
			return err
		}
		// Friend exclusion runs first so only one result set is open at a time.
		friends := map[int64]bool{}
		frows, err := qq.Query(ctx,
			`SELECT friend_id FROM social.friend_ids WHERE user_id=$1`, uid)
		if err != nil {
			return err
		}
		defer frows.Close()
		for frows.Next() {
			var f int64
			if err := frows.Scan(&f); err != nil {
				return err
			}
			friends[f] = true
		}
		if err := frows.Err(); err != nil {
			return err
		}
		// Blocked accounts are suppressed in both directions.
		blocked := map[int64]bool{}
		brows, err := qq.Query(ctx,
			`SELECT CASE WHEN blocker_id=$1 THEN blocked_id ELSE blocker_id END
			 FROM social.blocked_ids
			 WHERE blocker_id=$1 OR blocked_id=$1`, uid)
		if err != nil {
			return err
		}
		defer brows.Close()
		for brows.Next() {
			var b int64
			if err := brows.Scan(&b); err != nil {
				return err
			}
			blocked[b] = true
		}
		if err := brows.Err(); err != nil {
			return err
		}
		rankStart := time.Now()
		rows, err := qq.Query(ctx,
			`SELECT dp.user_id, dp.avatar_url,
			        ST_Distance(rp.geog, ST_SetSRID(ST_MakePoint($2,$3),4326)::geography)
			 FROM dating.radar_presence rp
			 JOIN dating.dating_profiles dp ON dp.user_id = rp.user_id
			 WHERE rp.user_id <> $1 AND NOT dp.is_hidden_from_friends
			   AND rp.updated_at > now() - interval '15 minutes'
			   AND ST_DWithin(rp.geog, ST_SetSRID(ST_MakePoint($2,$3),4326)::geography, $4)
			 ORDER BY rp.geog <-> ST_SetSRID(ST_MakePoint($2,$3),4326)::geography
			 LIMIT 50`,
			uid, lng, lat, radius)
		if err != nil {
			return err
		}
		defer rows.Close()
		defer func() { s.Metrics.ObserveRank(time.Since(rankStart)) }()

		type candidate struct {
			id   int64
			av   string
			dist float64
		}
		var candidates []candidate
		for rows.Next() {
			var c candidate
			if err := rows.Scan(&c.id, &c.av, &c.dist); err != nil {
				return err
			}
			if !friends[c.id] && !blocked[c.id] {
				candidates = append(candidates, c)
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(candidates) < radarKAnonymity {
			degraded = true
			nodes = []radarNode{}
			return nil
		}
		nodes = make([]radarNode, 0, len(candidates))
		for _, c := range candidates {
			nodes = append(nodes, radarNode{
				NodeID:   nodeIDEncode(c.id),
				Handle:   anonymousHandle(c.id),
				Avatar:   c.av,
				Distance: distanceBucket(c.dist),
			})
		}
		return nil
	})
	if err != nil {
		serverError(w, "radar scan", err)
		return
	}
	resp := map[string]any{"nodes": nodes, "degraded": degraded, "k_anonymity": radarKAnonymity}
	if degraded {
		resp["message"] = "Fewer than 3 active users nearby — results hidden to protect member privacy."
	}
	writeJSON(w, http.StatusOK, resp)
}

const radarKAnonymity = 3

// handleSwipe records a like/pass and creates a match on mutual like.
func (s *Server) handleSwipe(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdult(w, r) {
		return
	}
	uid, _ := userID(r)
	var req struct {
		NodeID string `json:"node_id"`
		Action string `json:"action"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	if req.Action != "like" && req.Action != "pass" {
		errorJSON(w, http.StatusBadRequest, "action must be like or pass")
		return
	}
	target, ok := nodeIDDecode(req.NodeID)
	if !ok || target == uid {
		errorJSON(w, http.StatusBadRequest, "invalid node_id")
		return
	}
	matched := false
	err := s.Store.DatingConn(r.Context(), func(q store.Querier) error {
		ctx := r.Context()
		var exists bool
		if err := q.QueryRow(ctx,
			`SELECT EXISTS (
			   SELECT 1 FROM dating.dating_profiles dp
			   WHERE dp.user_id=$1 AND NOT dp.is_hidden_from_friends
			     AND NOT EXISTS (
			       SELECT 1 FROM social.blocked_ids b
			       WHERE (b.blocker_id=$2 AND b.blocked_id=dp.user_id)
			          OR (b.blocker_id=dp.user_id AND b.blocked_id=$2)))`,
			target, uid).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errNotFound
		}
		if _, err := q.Exec(ctx,
			`INSERT INTO dating.swipes (user_id, target_id, action) VALUES ($1,$2,$3)
			 ON CONFLICT (user_id, target_id) DO UPDATE SET action=EXCLUDED.action, created_at=now()`,
			uid, target, req.Action); err != nil {
			return err
		}
		if req.Action == "like" {
			var mutual bool
			if err := q.QueryRow(ctx,
				`SELECT EXISTS (SELECT 1 FROM dating.swipes
				 WHERE user_id=$1 AND target_id=$2 AND action='like')`, target, uid).Scan(&mutual); err != nil {
				return err
			}
			if mutual {
				if _, err := q.Exec(ctx,
					`INSERT INTO dating.matches (a_id, b_id)
				 VALUES (LEAST($1::bigint,$2::bigint), GREATEST($1::bigint,$2::bigint))
				 ON CONFLICT (a_id, b_id) DO NOTHING`, uid, target); err != nil {
					return err
				}
				matched = true
				// Send match notifications to both users
				_ = s.NotifStore.Create(ctx, &store.Notification{
					UserID:   uid,
					Type:     "match",
					Title:    "It's a Match! 🎉",
					Body:     "You and someone liked each other. Start chatting!",
					ActorID:  &target,
				})
				_ = s.NotifStore.Create(ctx, &store.Notification{
					UserID:   target,
					Type:     "match",
					Title:    "It's a Match! 🎉",
					Body:     "You and someone liked each other. Start chatting!",
					ActorID:  &uid,
				})
			}
		}
		return nil
	})
	if err == errNotFound {
		errorJSON(w, http.StatusNotFound, "dating profile not available")
		return
	}
	if err != nil {
		serverError(w, "swipe", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"matched": matched})
}

type whisperPayload struct {
	From int64  `json:"from"`
	To   int64  `json:"to"`
	Body string `json:"body"`
}

// handleWhisperSend stores the message ONLY in Redis with a short TTL — the
// body never touches the database. It requires an existing mutual match.
func (s *Server) handleWhisperSend(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdult(w, r) {
		return
	}
	uid, _ := userID(r)
	var req struct {
		To   string `json:"to"`
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
	target, ok := nodeIDDecode(req.To)
	if !ok || target == uid {
		errorJSON(w, http.StatusBadRequest, "invalid to node_id")
		return
	}
	var matched, blocked bool
	err := s.Store.DatingConn(r.Context(), func(q store.Querier) error {
		ctx := r.Context()
		if err := q.QueryRow(ctx, `SELECT `+blockPairExpr, uid, target).Scan(&blocked); err != nil {
			return err
		}
		if blocked {
			return nil
		}
		return q.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM dating.matches
			 WHERE a_id = LEAST($1::bigint,$2::bigint)
			   AND b_id = GREATEST($1::bigint,$2::bigint))`, uid, target).Scan(&matched)
	})
	if err != nil {
		serverError(w, "check match", err)
		return
	}
	if blocked {
		errorJSON(w, http.StatusForbidden, "user is blocked")
		return
	}
	if !matched {
		errorJSON(w, http.StatusForbidden, "whisper requires a mutual match")
		return
	}
	buf := make([]byte, 16)
	if _, err := cryptoRead(buf); err != nil {
		serverError(w, "generate whisper id", err)
		return
	}
	id := hex.EncodeToString(buf)
	payload, _ := json.Marshal(whisperPayload{From: uid, To: target, Body: req.Body})
	if err := s.Store.RDB.Set(r.Context(), "whisper:"+id, payload,
		time.Duration(s.Cfg.WhisperTTLSeconds)*time.Second).Err(); err != nil {
		serverError(w, "store whisper", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "expires_in": s.Cfg.WhisperTTLSeconds})
}

// handleWhisperRead returns the message once and deletes it immediately
// ("auto-delete immediately after read"); TTL is the backstop.
func (s *Server) handleWhisperRead(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdult(w, r) {
		return
	}
	uid, _ := userID(r)
	id := r.PathValue("id")
	if id == "" || len(id) > 64 {
		errorJSON(w, http.StatusNotFound, "whisper not found")
		return
	}
	key := "whisper:" + id
	wipeStart := time.Now()
	val, err := s.Store.RDB.Get(r.Context(), key).Result()
	if err == redis.Nil {
		errorJSON(w, http.StatusGone, "whisper expired or already read")
		return
	}
	if err != nil {
		serverError(w, "read whisper", err)
		return
	}
	var p whisperPayload
	if err := json.Unmarshal([]byte(val), &p); err != nil {
		errorJSON(w, http.StatusGone, "whisper expired")
		return
	}
	if p.To != uid {
		_ = s.Store.RDB.Del(r.Context(), key).Err()
		s.Metrics.ObserveWhisperWipe(time.Since(wipeStart))
		errorJSON(w, http.StatusForbidden, "whisper is not addressed to you")
		return
	}
	_ = s.Store.RDB.Del(r.Context(), key).Err()
	s.Metrics.ObserveWhisperWipe(time.Since(wipeStart))
	writeJSON(w, http.StatusOK, map[string]any{"from": p.From, "body": p.Body, "deleted": true})
}
