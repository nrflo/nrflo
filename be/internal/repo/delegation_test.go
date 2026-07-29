package repo

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"be/internal/clock"
	"be/internal/model"
)

func TestDelegation_CreateGetRoundTrip(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	d := &model.Delegation{
		ID:                 "wfi-1.abcd1234",
		CallerSessionID:    "caller-sess",
		WorkflowInstanceID: "wfi-1",
		ProjectID:          "proj-1",
		Tier:               "extractor",
		Brief:              "extract the version",
		Fanout:             3,
		Depth:              1,
	}
	if err := r.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CallerSessionID != d.CallerSessionID || got.WorkflowInstanceID != d.WorkflowInstanceID ||
		got.ProjectID != d.ProjectID || got.Tier != d.Tier || got.Brief != d.Brief {
		t.Errorf("got = %+v, want fields matching seed %+v", got, d)
	}
	if got.Fanout != 3 {
		t.Errorf("Fanout = %d, want 3", got.Fanout)
	}
	if got.Depth != 1 {
		t.Errorf("Depth = %d, want 1", got.Depth)
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want running", got.Status)
	}
	if got.FanoutDone {
		t.Error("FanoutDone = true, want false on a fresh row")
	}
	if len(got.WorkerSessionIDs) != 3 || got.WorkerSessionIDs[0] != "" || got.WorkerSessionIDs[1] != "" || got.WorkerSessionIDs[2] != "" {
		t.Errorf("WorkerSessionIDs = %+v, want [\"\",\"\",\"\"] (pre-sized blank slots)", got.WorkerSessionIDs)
	}
	if len(got.SpawnErrors) != 3 {
		t.Errorf("SpawnErrors = %+v, want length-3 pre-sized array", got.SpawnErrors)
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt = %v, want nil on a fresh row", got.CompletedAt)
	}
	if got.ConsumedAt != nil {
		t.Errorf("ConsumedAt = %v, want nil on a fresh row", got.ConsumedAt)
	}
}

func TestDelegation_Get_UnknownID_ReturnsErrNoRows(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	_, err := r.Get("does-not-exist")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Get() error = %v, want sql.ErrNoRows", err)
	}
}

// TestDelegation_SetWorkerSlot_ConcurrentGoroutines is the lost-update
// regression test: every fanout worker calls SetWorkerSlot on its own index
// concurrently, and every slot must land intact — none clobbered by another
// worker's write (per-index json_set + SQLite statement serialization).
func TestDelegation_SetWorkerSlot_ConcurrentGoroutines(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	const fanout = 8
	d := &model.Delegation{
		ID:                 "wfi-2.concur01",
		CallerSessionID:    "caller-sess",
		WorkflowInstanceID: "wfi-2",
		ProjectID:          "proj-1",
		Tier:               "executor",
		Fanout:             fanout,
		Depth:              1,
	}
	if err := r.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < fanout; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sid := "worker-sess-" + string(rune('a'+idx))
			if err := r.SetWorkerSlot(d.ID, idx, sid, ""); err != nil {
				t.Errorf("SetWorkerSlot(%d): %v", idx, err)
			}
		}(i)
	}
	wg.Wait()

	got, err := r.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.WorkerSessionIDs) != fanout {
		t.Fatalf("len(WorkerSessionIDs) = %d, want %d", len(got.WorkerSessionIDs), fanout)
	}
	for i := 0; i < fanout; i++ {
		want := "worker-sess-" + string(rune('a'+i))
		if got.WorkerSessionIDs[i] != want {
			t.Errorf("WorkerSessionIDs[%d] = %q, want %q (lost update)", i, got.WorkerSessionIDs[i], want)
		}
	}
}

func TestDelegation_SetWorkerSlot_RecordsSpawnError(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	d := &model.Delegation{ID: "wfi-3.spawnerr", CallerSessionID: "c", WorkflowInstanceID: "wfi-3", ProjectID: "proj-1", Tier: "executor", Fanout: 2, Depth: 1}
	if err := r.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := r.SetWorkerSlot(d.ID, 0, "", "provider build failure: no api key"); err != nil {
		t.Fatalf("SetWorkerSlot: %v", err)
	}
	if err := r.SetWorkerSlot(d.ID, 1, "worker-ok", ""); err != nil {
		t.Fatalf("SetWorkerSlot: %v", err)
	}

	got, err := r.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.WorkerSessionIDs[0] != "" || got.SpawnErrors[0] != "provider build failure: no api key" {
		t.Errorf("slot 0 = session %q error %q, want empty session + spawn error", got.WorkerSessionIDs[0], got.SpawnErrors[0])
	}
	if got.WorkerSessionIDs[1] != "worker-ok" || got.SpawnErrors[1] != "" {
		t.Errorf("slot 1 = session %q error %q, want worker-ok + no error", got.WorkerSessionIDs[1], got.SpawnErrors[1])
	}
}

func TestDelegation_Create_SeedsBlankArraysAsValidJSON(t *testing.T) {
	t.Parallel()
	pool := newTestPool(t)
	r := NewDelegationRepo(pool, clock.Real())

	d := &model.Delegation{ID: "wfi-8.json01", CallerSessionID: "c", WorkflowInstanceID: "wfi-8", ProjectID: "proj-1", Tier: "executor", Fanout: 2, Depth: 1}
	if err := r.Create(d); err != nil {
		t.Fatalf("Create: %v", err)
	}

	var raw string
	if err := pool.QueryRow(`SELECT worker_session_ids FROM delegations WHERE id = ?`, d.ID).Scan(&raw); err != nil {
		t.Fatalf("select raw worker_session_ids: %v", err)
	}
	var arr []string
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		t.Fatalf("worker_session_ids not valid JSON: %v", err)
	}
	if len(arr) != 2 {
		t.Errorf("len(arr) = %d, want 2", len(arr))
	}
}
