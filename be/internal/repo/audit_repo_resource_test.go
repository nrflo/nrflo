package repo

import (
	"fmt"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/model"

	"github.com/google/uuid"
)

// TestAuditRepo_List_FilterByResourceTypeAndID covers the console tool audit
// query shape: GET /api/v1/audit-log?resource_type=agent_session&resource_id=<sid>.
func TestAuditRepo_List_FilterByResourceTypeAndID(t *testing.T) {
	t.Parallel()
	base := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	clk := clock.NewTest(base)
	r := setupAuditRepoWithClock(t, clk)

	action := "console.tool.call"
	sessionID := "sess-" + uuid.New().String()[:8]
	otherSessionID := "sess-other-" + uuid.New().String()[:8]

	for i := 0; i < 2; i++ {
		if err := r.Append(&model.AuditEntry{
			ID: fmt.Sprintf("ar-match-%d", i), Action: action,
			ResourceType: "agent_session", ResourceID: sessionID,
		}); err != nil {
			t.Fatalf("Append matching entry %d: %v", i, err)
		}
	}
	if err := r.Append(&model.AuditEntry{
		ID: "ar-other-session", Action: action,
		ResourceType: "agent_session", ResourceID: otherSessionID,
	}); err != nil {
		t.Fatalf("Append other-session entry: %v", err)
	}
	if err := r.Append(&model.AuditEntry{
		ID: "ar-other-type", Action: action,
		ResourceType: "project", ResourceID: sessionID,
	}); err != nil {
		t.Fatalf("Append other-type entry: %v", err)
	}

	entries, total, err := r.List(model.AuditFilter{ResourceType: "agent_session", ResourceID: sessionID}, 1, 100)
	if err != nil {
		t.Fatalf("List with ResourceType+ResourceID filter: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	for _, e := range entries {
		if e.ResourceType != "agent_session" || e.ResourceID != sessionID {
			t.Errorf("entry %s = (%q,%q), want (agent_session,%s)", e.ID, e.ResourceType, e.ResourceID, sessionID)
		}
	}
}
