package repo

import (
	"be/internal/clock"
	"be/internal/model"
	"testing"
)

func TestInboxScopePaginationAndReadState(t *testing.T) {
	t.Parallel()
	database := newTestDB(t)
	r := NewNotificationInboxRepo(database, clock.Real())
	var notes []*model.InboxNotification
	for _, project := range []string{"a", "b", "", "a"} {
		n := &model.InboxNotification{ProjectID: project, EventType: "orchestration.completed", Payload: []byte(`{"message":"done"}`)}
		if err := r.Insert(n); err != nil {
			t.Fatal(err)
		}
		notes = append(notes, n)
	}
	a := "A"
	result, err := r.List(&a, 1, 0)
	if err != nil || result.Total != 2 || result.Unread != 2 || len(result.Notifications) != 1 {
		t.Fatalf("project page: %+v, %v", result, err)
	}
	if err := r.MarkRead(&a, notes[1].ID); err != nil {
		t.Fatal(err)
	}
	if count, _ := r.CountUnread(); count != 4 {
		t.Fatal("cross-project mark changed read state")
	}
	if err := r.MarkRead(&a, notes[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := r.MarkRead(&a, ""); err != nil {
		t.Fatal(err)
	}
	if count, _ := r.CountUnread(); count != 2 {
		t.Fatalf("project mark-all: %d unread", count)
	}
	result, err = r.List(nil, 2, 2)
	if err != nil || result.Total != 4 || len(result.Notifications) != 2 {
		t.Fatalf("combined page: %+v, %v", result, err)
	}
	if err := r.MarkRead(nil, ""); err != nil {
		t.Fatal(err)
	}
	if count, _ := r.CountUnread(); count != 0 {
		t.Fatalf("global mark-all: %d unread", count)
	}
	result, err = r.List(&a, 50, 0)
	if err != nil || result.Notifications[0].ReadAt == nil {
		t.Fatalf("read state not persisted: %+v, %v", result, err)
	}
}
