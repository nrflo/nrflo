package service

import (
	"context"
	"io"
	"strings"
	"testing"

	"be/internal/types"
	"be/internal/ws"
)

func TestArtifact_Delete_RemovesStorageAndDB(t *testing.T) {
	svc, hub, _ := newArtifactSvcEnv(t)
	ctx := context.Background()
	seedProjWFI(t, svc.pool, "proj-del", "wfi-del")

	uid := stageOne(t, svc, "proj-del", "todel.bin", "bye")
	if err := svc.AttachInputArtifacts(ctx, "proj-del", "wfi-del", []types.InputArtifactRef{{UploadID: uid}}); err != nil {
		t.Fatalf("AttachInputArtifacts: %v", err)
	}

	artifacts, _ := svc.List(ctx, "wfi-del")
	if len(artifacts) == 0 {
		t.Fatal("no artifact to delete")
	}
	aid := artifacts[0].ID

	if err := svc.Delete(ctx, aid); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// DB row removed
	got, err := svc.Get(ctx, aid)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Error("artifact should not exist in DB after delete")
	}

	// WS event broadcast
	var found bool
	for _, e := range hub.events {
		if e.Type == ws.EventArtifactDeleted {
			found = true
			if e.Data["artifact_id"] != aid {
				t.Errorf("broadcast artifact_id = %v, want %q", e.Data["artifact_id"], aid)
			}
			if e.Data["workflow_instance_id"] != "wfi-del" {
				t.Errorf("broadcast workflow_instance_id = %v, want wfi-del", e.Data["workflow_instance_id"])
			}
		}
	}
	if !found {
		t.Errorf("%s event not broadcast; got events: %v", ws.EventArtifactDeleted, hub.events)
	}
}

func TestArtifact_Delete_NotFound(t *testing.T) {
	svc, _, _ := newArtifactSvcEnv(t)
	err := svc.Delete(context.Background(), "no-such-artifact")
	if err == nil {
		t.Error("expected error for nonexistent artifact")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found'", err.Error())
	}
}

func TestArtifact_List_OrderedByCreatedAt(t *testing.T) {
	svc, _, _ := newArtifactSvcEnv(t)
	ctx := context.Background()
	seedProjWFI(t, svc.pool, "proj-list", "wfi-list")

	for _, name := range []string{"alpha.txt", "beta.txt", "gamma.txt"} {
		uid := stageOne(t, svc, "proj-list", name, name)
		if err := svc.AttachInputArtifacts(ctx, "proj-list", "wfi-list", []types.InputArtifactRef{{UploadID: uid}}); err != nil {
			t.Fatalf("AttachInputArtifacts(%s): %v", name, err)
		}
	}

	artifacts, err := svc.List(ctx, "wfi-list")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(artifacts) != 3 {
		t.Fatalf("artifacts len = %d, want 3", len(artifacts))
	}
	names := []string{artifacts[0].Name, artifacts[1].Name, artifacts[2].Name}
	want := []string{"alpha.txt", "beta.txt", "gamma.txt"}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("artifact[%d].Name = %q, want %q", i, names[i], w)
		}
	}
}

func TestArtifact_List_Empty(t *testing.T) {
	svc, _, _ := newArtifactSvcEnv(t)
	artifacts, err := svc.List(context.Background(), "nonexistent-wfi")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(artifacts) != 0 {
		t.Errorf("artifacts len = %d, want 0", len(artifacts))
	}
}

func TestArtifact_Open_ReadsContent(t *testing.T) {
	svc, _, _ := newArtifactSvcEnv(t)
	ctx := context.Background()
	seedProjWFI(t, svc.pool, "proj-open", "wfi-open")

	wantContent := "open this content"
	uid := stageOne(t, svc, "proj-open", "open.txt", wantContent)
	if err := svc.AttachInputArtifacts(ctx, "proj-open", "wfi-open", []types.InputArtifactRef{{UploadID: uid}}); err != nil {
		t.Fatalf("AttachInputArtifacts: %v", err)
	}

	artifacts, _ := svc.List(ctx, "wfi-open")
	if len(artifacts) == 0 {
		t.Fatal("no artifact to open")
	}

	rc, err := svc.Open(ctx, artifacts[0])
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != wantContent {
		t.Errorf("content = %q, want %q", got, wantContent)
	}
}

func TestArtifact_Get_ReturnsNilForMissing(t *testing.T) {
	svc, _, _ := newArtifactSvcEnv(t)
	got, err := svc.Get(context.Background(), "no-such-id")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Error("Get(missing) should return nil, nil")
	}
}
