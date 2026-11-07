package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"

	"github.com/jackc/pgx/v5"
	"time"

	"nexus/backend/internal/auth"
)

var phoneRe = regexp.MustCompile(`^\+?[0-9]{7,15}$`)

const otpTTL = 5 * time.Minute

type credentials struct {
	Phone       string `json:"phone"`
	DisplayName string `json:"display_name"`
}

type verifyRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type userJSON struct {
	ID          int64  `json:"id"`
	Phone       string `json:"phone"`
	DisplayName string `json:"display_name"`
	Verified    bool   `json:"phone_verified"`
}

type tokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	TokenType    string   `json:"token_type"`
	User         userJSON `json:"user"`
}

func otpKey(phone string) string { return "otp:" + phone }

// issueOTP stores a fresh 6-digit code in Redis with a 5 minute TTL.
func (s *Server) issueOTP(ctx context.Context, phone string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	if err := s.Store.RDB.Set(ctx, otpKey(phone), code, otpTTL).Err(); err != nil {
		return "", err
	}
	return code, nil
}

// deliverOTP stores a code and hands it to the configured SMS sender. A
// failed delivery removes the stored code so the user's retry issues a fresh
// one instead of leaving an undeliverable code alive.
func (s *Server) deliverOTP(ctx context.Context, phone string) (string, error) {
	code, err := s.issueOTP(ctx, phone)
	if err != nil {
		return "", err
	}
	if err := s.SMS.SendOTP(ctx, phone, code); err != nil {
		_ = s.Store.RDB.Del(ctx, otpKey(phone)).Err()
		return "", err
	}
	return code, nil
}

// handleRegister upserts a user by phone and starts the OTP flow.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	if !phoneRe.MatchString(req.Phone) {
		errorJSON(w, http.StatusBadRequest, "phone must be 7-15 digits with optional + prefix")
		return
	}
	req.DisplayName = trimLimit(req.DisplayName, 80)
	if len(req.DisplayName) < 2 {
		errorJSON(w, http.StatusBadRequest, "display_name must be at least 2 characters")
		return
	}
	var id int64
	if err := s.Store.Social.QueryRow(r.Context(),
		`INSERT INTO social.users (phone, display_name) VALUES ($1, $2)
		 ON CONFLICT (phone) DO UPDATE SET display_name = EXCLUDED.display_name
		 RETURNING id`,
		req.Phone, req.DisplayName).Scan(&id); err != nil {
		serverError(w, "register user", err)
		return
	}
	code, err := s.deliverOTP(r.Context(), req.Phone)
	if err != nil {
		providerError(w, "send otp sms", "could not send verification code", err)
		return
	}
	resp := map[string]any{"user_id": id, "phone": req.Phone, "otp_sent": true}
	if s.Cfg.OTPDebug {
		resp["dev_code"] = code
	}
	writeJSON(w, http.StatusCreated, resp)
}

// handleLogin starts the OTP flow for an existing user.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req credentials
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	if !phoneRe.MatchString(req.Phone) {
		errorJSON(w, http.StatusBadRequest, "invalid phone format")
		return
	}
	var id int64
	err := s.Store.Social.QueryRow(r.Context(),
		`SELECT id FROM social.users WHERE phone=$1`, req.Phone).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorJSON(w, http.StatusNotFound, "user not found")
		} else {
			serverError(w, "lookup user", err)
		}
		return
	}
	code, err := s.deliverOTP(r.Context(), req.Phone)
	if err != nil {
		providerError(w, "send otp sms", "could not send verification code", err)
		return
	}
	resp := map[string]any{"user_id": id, "phone": req.Phone, "otp_sent": true}
	if s.Cfg.OTPDebug {
		resp["dev_code"] = code
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleVerify checks the OTP (constant-time), marks the phone verified and
// issues the access/refresh token pair.
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	if len(req.Code) != 6 || !isDigits(req.Code) {
		errorJSON(w, http.StatusBadRequest, "code must be 6 digits")
		return
	}
	stored, err := s.Store.RDB.GetDel(r.Context(), otpKey(req.Phone)).Result()
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "code expired or not requested")
		return
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(req.Code)) != 1 {
		errorJSON(w, http.StatusBadRequest, "invalid code")
		return
	}
	var u userJSON
	err = s.Store.Social.QueryRow(r.Context(),
		`UPDATE social.users SET phone_verified=true WHERE phone=$1
		 RETURNING id, phone, display_name, phone_verified`,
		req.Phone).Scan(&u.ID, &u.Phone, &u.DisplayName, &u.Verified)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorJSON(w, http.StatusNotFound, "user not found")
		} else {
			serverError(w, "verify user", err)
		}
		return
	}
	access, err := s.JWT.Sign(u.ID, "access", auth.AccessTTL)
	if err != nil {
		serverError(w, "sign access token", err)
		return
	}
	refresh, err := s.JWT.Sign(u.ID, "refresh", auth.RefreshTTL)
	if err != nil {
		serverError(w, "sign refresh token", err)
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		User:         u,
	})
}

// handleRefresh exchanges a valid refresh token for a new access token.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	claims, err := s.JWT.Verify(req.RefreshToken, "refresh")
	if err != nil {
		errorJSON(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	uid, err := auth.UserID(claims)
	if err != nil {
		errorJSON(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}
	access, err := s.JWT.Sign(uid, "access", auth.AccessTTL)
	if err != nil {
		serverError(w, "sign access token", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"access_token": access, "token_type": "Bearer"})
}

// handleMe returns the caller account with merged user profile fields.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var u userJSON
	var realName, township, avatar string
	var adult bool
	err := s.Store.Social.QueryRow(r.Context(),
		`SELECT u.id, COALESCE(u.phone,'') AS phone, u.display_name, u.phone_verified,
		        COALESCE(p.real_name,''), COALESCE(p.township,''), COALESCE(p.avatar_url,''),
		        u.birth_date IS NOT NULL
		          AND u.birth_date <= (CURRENT_DATE - INTERVAL '18 years')
		 FROM social.users u
		 LEFT JOIN social.user_profiles p ON p.user_id = u.id
		 WHERE u.id=$1`, uid).Scan(&u.ID, &u.Phone, &u.DisplayName, &u.Verified, &realName, &township, &avatar, &adult)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorJSON(w, http.StatusNotFound, "account not found")
		} else {
			serverError(w, "load account", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": u.ID, "phone": u.Phone, "display_name": u.DisplayName,
		"phone_verified": u.Verified, "real_name": realName,
		"township": township, "avatar_url": avatar, "adult": adult,
	})
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}
