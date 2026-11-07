package api

import (
	"net/http"

	"github.com/jackc/pgx/v5"
)

type profileRequest struct {
	RealName  string `json:"real_name"`
	Township  string `json:"township"`
	AvatarURL string `json:"avatar_url"`
}

// handleGetProfile returns the caller's user_profiles row (or defaults).
func (s *Server) handleGetProfile(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var realName, township, avatar string
	err := s.Store.Social.QueryRow(r.Context(),
		`SELECT COALESCE(real_name,''), COALESCE(township,''), COALESCE(avatar_url,'')
		 FROM social.user_profiles WHERE user_id=$1`, uid).
		Scan(&realName, &township, &avatar)
	if err == pgx.ErrNoRows {
		// No profile yet: return defaults so first-run UX works.
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id": uid, "real_name": "", "township": "", "avatar_url": "",
		})
		return
	}
	if err != nil {
		serverError(w, "read profile", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": uid, "real_name": realName, "township": township, "avatar_url": avatar,
	})
}

// handlePutProfile upserts the user_profiles row (main account only).
func (s *Server) handlePutProfile(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var req profileRequest
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	req.RealName = trimLimit(req.RealName, 120)
	req.Township = trimLimit(req.Township, 120)
	req.AvatarURL = trimLimit(req.AvatarURL, 500)
	if len(req.RealName) < 2 {
		errorJSON(w, http.StatusBadRequest, "real_name must be at least 2 characters")
		return
	}
	if _, err := s.Store.Social.Exec(r.Context(),
		`INSERT INTO social.user_profiles (user_id, real_name, township, avatar_url, updated_at)
		 VALUES ($1,$2,$3,$4, now())
		 ON CONFLICT (user_id) DO UPDATE
		 SET real_name=EXCLUDED.real_name, township=EXCLUDED.township,
		     avatar_url=EXCLUDED.avatar_url, updated_at=now()`,
		uid, req.RealName, req.Township, req.AvatarURL); err != nil {
		serverError(w, "save profile", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id": uid, "real_name": req.RealName,
		"township": req.Township, "avatar_url": req.AvatarURL,
	})
}
