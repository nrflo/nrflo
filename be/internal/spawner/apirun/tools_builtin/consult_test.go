package tools_builtin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"be/internal/spawner/apirun"
)

// Compile-time assertion: fakeConsultant satisfies apirun.ConsultantSpawner.
var _ apirun.ConsultantSpawner = (*fakeConsultant)(nil)

type fakeConsultant struct {
	callerSessionID string
	consultantID    string
	question        string
	answer          string
	err             error
}

func (f *fakeConsultant) Consult(_ context.Context, callerSessionID, consultantID, question string) (string, error) {
	f.callerSessionID = callerSessionID
	f.consultantID = consultantID
	f.question = question
	return f.answer, f.err
}

func TestConsult_HappyPath(t *testing.T) {
	env := newBuiltinTestEnv(t)
	fake := &fakeConsultant{answer: "the answer"}
	env.env.Consultant = fake

	out, isErr, err := invoke(t, env.env, "consult", `{"consultant":"architect","question":"how?"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if isErr {
		t.Errorf("isErr=true, want false")
	}
	if out != "the answer" {
		t.Errorf("output=%q, want %q", out, "the answer")
	}
	if fake.callerSessionID != testSessionID {
		t.Errorf("callerSessionID=%q, want %q", fake.callerSessionID, testSessionID)
	}
	if fake.consultantID != "architect" {
		t.Errorf("consultantID=%q, want architect", fake.consultantID)
	}
	if fake.question != "how?" {
		t.Errorf("question=%q, want how?", fake.question)
	}
}

func TestConsult_NilConsultant_ReturnsServiceError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	// Consultant is nil by default

	out, isErr, err := invoke(t, env.env, "consult", `{"consultant":"x","question":"y"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true")
	}
	if !strings.Contains(out, "consultant") {
		t.Errorf("output=%q, want contains 'consultant'", out)
	}
}

func TestConsult_ConsultantReturnsError_IsError(t *testing.T) {
	env := newBuiltinTestEnv(t)
	fake := &fakeConsultant{err: errors.New("internal failure")}
	env.env.Consultant = fake

	out, isErr, err := invoke(t, env.env, "consult", `{"consultant":"x","question":"y"}`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true")
	}
	if out != "internal failure" {
		t.Errorf("output=%q, want 'internal failure'", out)
	}
}

func TestConsult_InvalidJSON_InvalidArgs(t *testing.T) {
	env := newBuiltinTestEnv(t)
	fake := &fakeConsultant{}
	env.env.Consultant = fake

	out, isErr, err := invoke(t, env.env, "consult", `not-valid-json`)
	if err != nil {
		t.Fatalf("Invoke err: %v", err)
	}
	if !isErr {
		t.Errorf("isErr=false, want true")
	}
	if !strings.Contains(out, "invalid arguments") {
		t.Errorf("output=%q, want contains 'invalid arguments'", out)
	}
}
