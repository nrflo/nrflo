package repo

import (
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"

	"github.com/google/uuid"
)

type NotificationInboxRepo struct {
	db    db.Querier
	clock clock.Clock
}

func NewNotificationInboxRepo(database db.Querier, clk clock.Clock) *NotificationInboxRepo {
	return &NotificationInboxRepo{db: database, clock: clk}
}

func (r *NotificationInboxRepo) Insert(n *model.InboxNotification) error {
	n.ID = uuid.NewString()
	n.ProjectID = strings.ToLower(n.ProjectID)
	n.CreatedAt = r.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.db.Exec(`INSERT INTO notification_inbox
 (id, project_id, ticket_id, workflow_id, event_type, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.ProjectID, n.TicketID, n.WorkflowID, n.EventType, string(n.Payload), n.CreatedAt)
	return err
}

// A nil project selects the combined inbox; a pointer selects exactly one scope.
func (r *NotificationInboxRepo) List(project *string, limit, offset int) (*model.NotificationInbox, error) {
	where, args := inboxScope(project)
	result := &model.NotificationInbox{Notifications: []*model.InboxNotification{}}
	if err := r.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(read_at IS NULL), 0) FROM notification_inbox`+where, args...).Scan(&result.Total, &result.Unread); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	rows, err := r.db.Query(`SELECT id, project_id, ticket_id, workflow_id, event_type, payload, created_at, read_at
 FROM notification_inbox`+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		n := &model.InboxNotification{}
		var payload string
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.TicketID, &n.WorkflowID, &n.EventType, &payload, &n.CreatedAt, &n.ReadAt); err != nil {
			return nil, err
		}
		n.Payload = []byte(payload)
		result.Notifications = append(result.Notifications, n)
	}
	return result, rows.Err()
}

func (r *NotificationInboxRepo) CountUnread() (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM notification_inbox WHERE read_at IS NULL`).Scan(&count)
	return count, err
}

func (r *NotificationInboxRepo) MarkRead(project *string, id string) error {
	where, args := inboxScope(project)
	if where == "" {
		where = " WHERE 1=1"
	}
	if id != "" {
		where += " AND id = ?"
		args = append(args, id)
	}
	args = append([]any{r.clock.Now().UTC().Format(time.RFC3339Nano)}, args...)
	_, err := r.db.Exec(`UPDATE notification_inbox SET read_at = ?`+where+` AND read_at IS NULL`, args...)
	return err
}

func inboxScope(project *string) (string, []any) {
	if project == nil {
		return "", nil
	}
	return " WHERE project_id = ?", []any{strings.ToLower(*project)}
}

func (r *NotificationInboxRepo) Get(project *string, id string) (*model.InboxNotification, error) {
	where, args := inboxScope(project)
	if where == "" {
		where = " WHERE 1=1"
	}
	args = append(args, id)
	n := &model.InboxNotification{}
	var payload string
	err := r.db.QueryRow(`SELECT id, project_id, ticket_id, workflow_id, event_type, payload, created_at, read_at FROM notification_inbox`+where+` AND id = ?`, args...).Scan(&n.ID, &n.ProjectID, &n.TicketID, &n.WorkflowID, &n.EventType, &payload, &n.CreatedAt, &n.ReadAt)
	if err != nil {
		return nil, err
	}
	n.Payload = []byte(payload)
	return n, nil
}
