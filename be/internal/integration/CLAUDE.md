# Integration Tests Package

Integration tests using a shared test harness that creates an isolated stack per test.

## Using `NewTestEnv(t)`

All integration tests use `NewTestEnv(t)` from `testenv.go`. It creates an isolated stack with fresh DB, socket server, WS hub, and seeded project + workflow definition.

Data setup uses services directly (not socket), since the socket only handles agent/findings methods:

```go
func TestSomething(t *testing.T) {
    env := NewTestEnv(t) // fresh DB, socket, hub, project, services

    // Setup via service helpers
    env.CreateTicket(t, "TICKET-1", "Test ticket")
    env.InitWorkflow(t, "TICKET-1")
    env.StartPhase(t, "TICKET-1", "analyzer")

    // Socket calls for agent/findings
    env.MustExecute(t, "findings.add", map[string]interface{}{...}, nil)
    env.MustExecute(t, "agent.complete", map[string]interface{}{...}, nil)

    // WebSocket testing
    _, ch := env.NewWSClient(t, "client-id", "TICKET-1")
}
```

## Helper Methods

| Method | Purpose |
|--------|---------|
| `CreateTicket(t, id, title)` | Create ticket via service |
| `InitWorkflow(t, ticketID)` | Init "test" workflow via service |
| `StartPhase(t, ticketID, phase)` | Start phase via service |
| `CompletePhase(t, ticketID, phase, result)` | Complete phase via service |
| `MustExecute(t, method, params, &result)` | Call socket method (agent/findings only) |
| `ExpectError(t, method, params, code)` | Assert socket error response |
| `NewWSClient(t, id, ticketID)` | Create subscribed WS test client |
| `GetWorkflowInstanceID(t, ticketID, workflow)` | Get workflow instance UUID |
| `InsertAgentSession(t, ...)` | Insert agent session row directly |

Services are also available directly: `env.ProjectSvc`, `env.TicketSvc`, `env.WorkflowSvc`, `env.AgentSvc`, `env.FindingsSvc`.

`env.Clock` is a `*clock.TestClock` initialized to `2025-01-01T00:00:00Z`. Use `env.Clock.Advance(d)` to move time forward for timestamp differentiation instead of `time.Sleep`.

## Key Gotchas

- **Socket path limit**: macOS has 104-char limit. `NewTestEnv` uses `/tmp/nrwf-it-*.sock`
- **Server stop hangs**: Cleanup uses 2-sec timeout context to avoid blocking
- **No config file needed**: Agent config comes from DB agent_definitions, not file-based config

## Running Tests

```bash
cd be
make test                    # all tests
make test-integration        # integration only (verbose)
./scripts/test.sh -c         # with coverage
./scripts/test.sh -r         # with race detector
./scripts/test.sh -i -v -r   # combine flags
```

## Test Files

| File | Tests |
|------|-------|
| `testenv.go` | Shared test harness (`NewTestEnv`) |
| `workflow_test.go` | Workflow init, phases, set/get (via service) |
| `findings_test.go` | Findings add/append/delete, models (via socket) |
| `agent_test.go` | Agent complete/fail/continue (via socket) |
| `websocket_test.go` | WS broadcast, subscription filtering |
| `messages_test.go` | Agent message storage, pagination (via service) |
| `chain_epic_test.go` | Chain epic auto-close on completion |
| `run_epic_workflow_test.go` | Run-epic endpoint: happy path, errors, closed children |
| `error_test.go` | Error codes, validation |

## When to Add Tests

- Agent/findings socket changes → add integration test in appropriate `*_test.go`
- Service logic changes → verify existing tests still pass, add cases for new behavior
- Bug fixes → add regression test
