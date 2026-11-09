package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Notification represents an in-app notification.
type Notification struct {
	ID        int64
	UserID    int64
	Type      string
	Title     string
	Body      string
	ActorID   *int64
	TargetID  *int64
	Read      bool
	CreatedAt time.Time
}

// NotificationStore manages notifications in the social pool.
type NotificationStore struct {
	pool *pgxpool.Pool
}

// NewNotificationStore creates a new notification store.
func NewNotificationStore(pool *pgxpool.Pool) *NotificationStore {
	return &NotificationStore{pool: pool}
}

// ListByUser returns notifications for a user, newest first.
func (s *NotificationStore) ListByUser(ctx context.Context, userID int64, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, type, title, body, actor_id, target_id, read, created_at
		FROM social.notifications
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body,
			&n.ActorID, &n.TargetID, &n.Read, &n.CreatedAt); err != nil {
			return nil, err
		}
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

// MarkRead marks a notification as read.
func (s *NotificationStore) MarkRead(ctx context.Context, userID, notificationID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE social.notifications
		SET read = true
		WHERE id = $1 AND user_id = $2`, notificationID, userID)
	return err
}

// MarkAllRead marks all notifications as read for a user.
func (s *NotificationStore) MarkAllRead(ctx context.Context, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE social.notifications
		SET read = true
		WHERE user_id = $1 AND read = false`, userID)
	return err
}

// UnreadCount returns the number of unread notifications for a user.
func (s *NotificationStore) UnreadCount(ctx context.Context, userID int64) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM social.notifications
		WHERE user_id = $1 AND read = false`, userID).Scan(&count)
	return count, err
}

// Create inserts a new notification.
func (s *NotificationStore) Create(ctx context.Context, n *Notification) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO social.notifications (user_id, type, title, body, actor_id, target_id)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`,
		n.UserID, n.Type, n.Title, n.Body, n.ActorID, n.TargetID).Scan(&n.ID, &n.CreatedAt)
}

// DeleteOld removes notifications older than the given duration.
func (s *NotificationStore) DeleteOld(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)
	_, err := s.pool.Exec(ctx, `
		DELETE FROM social.notifications
		WHERE created_at < $1 AND read = true`, cutoff)
	return err
}
