package api

import (
	"net/http"
	"strconv"
	"time"

	"nexus/backend/internal/store"
)

type notificationJSON struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	ActorID   *int64    `json:"actor_id,omitempty"`
	TargetID  *int64    `json:"target_id,omitempty"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

// handleListNotifications returns the caller's notifications.
func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	notifications, err := s.NotifStore.ListByUser(r.Context(), uid, limit)
	if err != nil {
		serverError(w, "list notifications", err)
		return
	}

	out := make([]notificationJSON, 0, len(notifications))
	for _, n := range notifications {
		out = append(out, notificationToJSON(n))
	}

	unread, _ := s.NotifStore.UnreadCount(r.Context(), uid)
	writeJSON(w, http.StatusOK, map[string]any{
		"notifications": out,
		"unread_count":  unread,
	})
}

// handleMarkNotificationRead marks a single notification as read.
func (s *Server) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid notification id")
		return
	}

	if err := s.NotifStore.MarkRead(r.Context(), uid, id); err != nil {
		serverError(w, "mark notification read", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleMarkAllNotificationsRead marks all notifications as read.
func (s *Server) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	if err := s.NotifStore.MarkAllRead(r.Context(), uid); err != nil {
		serverError(w, "mark all notifications read", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func notificationToJSON(n store.Notification) notificationJSON {
	return notificationJSON{
		ID:        n.ID,
		Type:      n.Type,
		Title:     n.Title,
		Body:      n.Body,
		ActorID:   n.ActorID,
		TargetID:  n.TargetID,
		Read:      n.Read,
		CreatedAt: n.CreatedAt,
	}
}
