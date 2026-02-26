package model

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"
)

// assertTicketRoundtrip compares two tickets field by field after a marshal/unmarshal cycle.
func assertTicketRoundtrip(t *testing.T, want, got Ticket) {
	t.Helper()

	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.ProjectID != want.ProjectID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, want.ProjectID)
	}
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %q, want %q", got.Status, want.Status)
	}
	if got.Priority != want.Priority {
		t.Errorf("Priority = %d, want %d", got.Priority, want.Priority)
	}
	if got.IssueType != want.IssueType {
		t.Errorf("IssueType = %q, want %q", got.IssueType, want.IssueType)
	}
	if got.CreatedBy != want.CreatedBy {
		t.Errorf("CreatedBy = %q, want %q", got.CreatedBy, want.CreatedBy)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want.CreatedAt)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, want.UpdatedAt)
	}

	if got.Description.Valid != want.Description.Valid {
		t.Errorf("Description.Valid = %v, want %v", got.Description.Valid, want.Description.Valid)
	} else if want.Description.Valid && got.Description.String != want.Description.String {
		t.Errorf("Description.String = %q, want %q", got.Description.String, want.Description.String)
	}

	if got.ParentTicketID.Valid != want.ParentTicketID.Valid {
		t.Errorf("ParentTicketID.Valid = %v, want %v", got.ParentTicketID.Valid, want.ParentTicketID.Valid)
	} else if want.ParentTicketID.Valid && got.ParentTicketID.String != want.ParentTicketID.String {
		t.Errorf("ParentTicketID.String = %q, want %q", got.ParentTicketID.String, want.ParentTicketID.String)
	}

	// JSON RFC3339 has second precision — truncate before comparing.
	if got.ClosedAt.Valid != want.ClosedAt.Valid {
		t.Errorf("ClosedAt.Valid = %v, want %v", got.ClosedAt.Valid, want.ClosedAt.Valid)
	} else if want.ClosedAt.Valid {
		wantTime := want.ClosedAt.Time.Truncate(time.Second)
		gotTime := got.ClosedAt.Time.Truncate(time.Second)
		if !gotTime.Equal(wantTime) {
			t.Errorf("ClosedAt.Time = %v, want %v", gotTime, wantTime)
		}
	}

	if got.CloseReason.Valid != want.CloseReason.Valid {
		t.Errorf("CloseReason.Valid = %v, want %v", got.CloseReason.Valid, want.CloseReason.Valid)
	} else if want.CloseReason.Valid && got.CloseReason.String != want.CloseReason.String {
		t.Errorf("CloseReason.String = %q, want %q", got.CloseReason.String, want.CloseReason.String)
	}
}

func TestTicketJSONRoundtrip(t *testing.T) {
	closedAt := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name   string
		ticket Ticket
	}{
		{
			name: "all nullable fields populated",
			ticket: Ticket{
				ID:             "TICKET-1",
				ProjectID:      "proj-1",
				Title:          "Test ticket",
				Description:    sql.NullString{String: "A description", Valid: true},
				Status:         StatusOpen,
				Priority:       2,
				IssueType:      IssueTypeFeature,
				ParentTicketID: sql.NullString{String: "TICKET-0", Valid: true},
				CreatedAt:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:      time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
				ClosedAt:       sql.NullTime{Time: closedAt, Valid: true},
				CreatedBy:      "user1",
				CloseReason:    sql.NullString{String: "resolved", Valid: true},
			},
		},
		{
			name: "all nullable fields null",
			ticket: Ticket{
				ID:        "TICKET-2",
				ProjectID: "proj-1",
				Title:     "Null fields ticket",
				Status:    StatusInProgress,
				Priority:  1,
				IssueType: IssueTypeBug,
				CreatedAt: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
				CreatedBy: "user2",
			},
		},
		{
			name: "description only, others null",
			ticket: Ticket{
				ID:          "TICKET-3",
				ProjectID:   "proj-2",
				Title:       "Partial ticket",
				Description: sql.NullString{String: "Only description set", Valid: true},
				Status:      StatusClosed,
				Priority:    3,
				IssueType:   IssueTypeTask,
				CreatedAt:   time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:   time.Date(2025, 5, 2, 0, 0, 0, 0, time.UTC),
				CreatedBy:   "user3",
			},
		},
		{
			name: "empty string description is not null",
			ticket: Ticket{
				ID:          "TICKET-4",
				ProjectID:   "proj-1",
				Title:       "Empty description",
				Description: sql.NullString{String: "", Valid: true},
				Status:      StatusOpen,
				Priority:    1,
				IssueType:   IssueTypeBug,
				CreatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				UpdatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				CreatedBy:   "user4",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.ticket)
			if err != nil {
				t.Fatalf("json.Marshal error: %v", err)
			}

			var got Ticket
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("json.Unmarshal error: %v", err)
			}

			assertTicketRoundtrip(t, tt.ticket, got)
		})
	}
}

// TestTicketMarshalJSONFormat verifies the wire format: null fields appear
// as JSON null (not omitted), set fields appear as their string/time values.
func TestTicketMarshalJSONFormat(t *testing.T) {
	t.Run("null fields produce JSON null", func(t *testing.T) {
		ticket := Ticket{
			ID:        "T-1",
			ProjectID: "p-1",
			Title:     "A ticket",
			Status:    StatusOpen,
			Priority:  1,
			IssueType: IssueTypeBug,
			CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			CreatedBy: "alice",
		}

		data, err := json.Marshal(ticket)
		if err != nil {
			t.Fatalf("json.Marshal error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("json.Unmarshal into map error: %v", err)
		}

		for _, key := range []string{"description", "parent_ticket_id", "closed_at", "close_reason"} {
			val, exists := m[key]
			if !exists {
				t.Errorf("key %q missing from JSON output", key)
				continue
			}
			if val != nil {
				t.Errorf("key %q = %v (%T), want null", key, val, val)
			}
		}
	})

	t.Run("set fields produce JSON values", func(t *testing.T) {
		desc := "my desc"
		parent := "TICKET-0"
		reason := "done"
		closedAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

		ticket := Ticket{
			ID:             "T-2",
			ProjectID:      "p-1",
			Title:          "A ticket",
			Description:    sql.NullString{String: desc, Valid: true},
			ParentTicketID: sql.NullString{String: parent, Valid: true},
			ClosedAt:       sql.NullTime{Time: closedAt, Valid: true},
			CloseReason:    sql.NullString{String: reason, Valid: true},
			Status:         StatusClosed,
			Priority:       1,
			IssueType:      IssueTypeFeature,
			CreatedAt:      time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
			CreatedBy:      "alice",
		}

		data, err := json.Marshal(ticket)
		if err != nil {
			t.Fatalf("json.Marshal error: %v", err)
		}

		var m map[string]interface{}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("json.Unmarshal into map error: %v", err)
		}

		if m["description"] != desc {
			t.Errorf("description = %v, want %q", m["description"], desc)
		}
		if m["parent_ticket_id"] != parent {
			t.Errorf("parent_ticket_id = %v, want %q", m["parent_ticket_id"], parent)
		}
		if m["close_reason"] != reason {
			t.Errorf("close_reason = %v, want %q", m["close_reason"], reason)
		}
		if m["closed_at"] == nil {
			t.Errorf("closed_at = null, want a time string")
		}
	})
}

// TestTicketUnmarshalJSONError verifies that malformed JSON returns an error.
func TestTicketUnmarshalJSONError(t *testing.T) {
	var ticket Ticket
	if err := json.Unmarshal([]byte(`{invalid json`), &ticket); err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// TestTicketUnmarshalJSONNullIndependence verifies that null and non-null
// fields are independent (setting one doesn't affect the others).
func TestTicketUnmarshalJSONNullIndependence(t *testing.T) {
	raw := `{
		"id":"T-3","project_id":"p-1","title":"test","status":"open",
		"priority":1,"issue_type":"bug",
		"created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z",
		"created_by":"alice",
		"description":"hello","parent_ticket_id":null,"closed_at":null,"close_reason":null
	}`

	var ticket Ticket
	if err := json.Unmarshal([]byte(raw), &ticket); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if !ticket.Description.Valid || ticket.Description.String != "hello" {
		t.Errorf("Description = {Valid:%v, String:%q}, want {true, \"hello\"}", ticket.Description.Valid, ticket.Description.String)
	}
	if ticket.ParentTicketID.Valid {
		t.Errorf("ParentTicketID.Valid = true, want false")
	}
	if ticket.ClosedAt.Valid {
		t.Errorf("ClosedAt.Valid = true, want false")
	}
	if ticket.CloseReason.Valid {
		t.Errorf("CloseReason.Valid = true, want false")
	}
}

// TestTicketSliceRoundtrip tests marshaling/unmarshaling a slice of tickets,
// matching the ticketsList CLI path that unmarshals []*model.Ticket.
func TestTicketSliceRoundtrip(t *testing.T) {
	tickets := []*Ticket{
		{
			ID:          "T-1",
			ProjectID:   "p-1",
			Title:       "First",
			Description: sql.NullString{String: "desc one", Valid: true},
			Status:      StatusOpen,
			Priority:    1,
			IssueType:   IssueTypeBug,
			CreatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			CreatedBy:   "alice",
		},
		{
			ID:          "T-2",
			ProjectID:   "p-1",
			Title:       "Second",
			CloseReason: sql.NullString{String: "done", Valid: true},
			ClosedAt:    sql.NullTime{Time: time.Date(2025, 2, 15, 9, 0, 0, 0, time.UTC), Valid: true},
			Status:      StatusClosed,
			Priority:    2,
			IssueType:   IssueTypeFeature,
			CreatedAt:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC),
			CreatedBy:   "bob",
		},
	}

	data, err := json.Marshal(tickets)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}

	var got []*Ticket
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}

	if len(got) != len(tickets) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(tickets))
	}

	for i := range tickets {
		t.Run(tickets[i].ID, func(t *testing.T) {
			assertTicketRoundtrip(t, *tickets[i], *got[i])
		})
	}
}
