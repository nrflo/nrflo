package client

import (
	"bufio"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestExecuteAndUnmarshalWithReadDeadline_ShortDeadlineTimesOut verifies that a
// custom read deadline is applied: when the server never responds the call returns
// an error well within the default 5-minute window.
func TestExecuteAndUnmarshalWithReadDeadline_ShortDeadlineTimesOut(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "clienttest")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sockPath := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	// Accept connections but never write a response.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				scanner := bufio.NewScanner(conn)
				for scanner.Scan() {
				}
			}()
		}
	}()

	c := NewWithAddr("unix", sockPath, "test-proj")
	var result struct{ Answer string }

	start := time.Now()
	err = c.ExecuteAndUnmarshalWithReadDeadline(
		"agent.consult",
		map[string]interface{}{"consultant": "c", "question": "q"},
		&result,
		20*time.Millisecond,
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 5*time.Second {
		t.Errorf("took %v, should have timed out well before 5 s with a 20 ms deadline", elapsed)
	}
}
