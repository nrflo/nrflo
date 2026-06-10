package repo

import (
	"sync"
	"testing"

	"be/internal/clock"
)

// TestAgentMessageRepo_InsertBatch_ConcurrentWritersUniqueSeq verifies the
// in-transaction seq assignment: concurrent batches on the same session
// (output flush vs hook events vs user input) must produce unique,
// contiguous seq values with no duplicates.
func TestAgentMessageRepo_InsertBatch_ConcurrentWritersUniqueSeq(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	const sessionID = "msg-sess-conc"
	for _, q := range []string{
		`INSERT INTO projects (id, name, created_at, updated_at) VALUES ('conc-proj', 'P', datetime('now'), datetime('now'))`,
		`INSERT INTO workflows (project_id, id, description, scope_type, created_at, updated_at) VALUES ('conc-proj', 'conc-wf', '', 'project', datetime('now'), datetime('now'))`,
		`INSERT INTO workflow_instances (id, project_id, ticket_id, workflow_id, status, scope_type, created_at, updated_at) VALUES ('conc-wfi', 'conc-proj', '', 'conc-wf', 'active', 'project', datetime('now'), datetime('now'))`,
		`INSERT INTO agent_sessions (id, project_id, ticket_id, workflow_instance_id, phase, agent_type, model_id, status, created_at, updated_at) VALUES ('` + sessionID + `', 'conc-proj', '', 'conc-wfi', 'ph', 'ag', 'm', 'running', datetime('now'), datetime('now'))`,
	} {
		if _, err := pool.Exec(q); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	r := NewAgentMessageRepo(pool, clock.Real())
	const writers = 8
	const batchesPerWriter = 5
	const rowsPerBatch = 2
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for b := 0; b < batchesPerWriter; b++ {
				err := r.InsertBatch(sessionID, []MessageEntry{
					{Content: "first"},
					{Content: "second"},
				})
				if err != nil {
					t.Errorf("InsertBatch: %v", err)
				}
			}
		}()
	}
	wg.Wait()

	rows, err := pool.Query(`SELECT seq FROM agent_messages WHERE session_id = ? ORDER BY seq ASC`, sessionID)
	if err != nil {
		t.Fatalf("query seqs: %v", err)
	}
	defer rows.Close()

	var seqs []int
	for rows.Next() {
		var s int
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		seqs = append(seqs, s)
	}

	total := writers * batchesPerWriter * rowsPerBatch
	if len(seqs) != total {
		t.Fatalf("got %d rows, want %d", len(seqs), total)
	}
	for i, s := range seqs {
		if s != i {
			t.Fatalf("seqs not contiguous/unique: seqs[%d] = %d (full: %v)", i, s, seqs)
		}
	}
}
