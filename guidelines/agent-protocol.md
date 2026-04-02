# Agent Protocol

Guidelines for prompt authors writing agent definition prompts.

## Agent Communication

Agents communicate with the nrflow server via CLI commands over Unix socket:

```bash
# Report results (exit 0 = pass, no explicit call needed)
nrflow agent fail <ticket> <agent-type> -w <workflow> [--reason <text>]
nrflow agent continue <ticket> <agent-type> -w <workflow>

# Store findings for downstream agents
nrflow findings add <ticket> <agent-type> key:value -w <workflow>
nrflow findings append <ticket> <agent-type> key:value -w <workflow>
nrflow findings get <ticket> <agent-type> [key] -w <workflow>
nrflow findings delete <ticket> <agent-type> <keys...> -w <workflow>
```

## Template Variables

Agent prompts support these variables, expanded by the spawner before the agent starts:

| Variable | Description |
|----------|-------------|
| `${AGENT}` | Agent type (e.g., "implementor") |
| `${TICKET_ID}` | Current ticket ID |
| `${TICKET_TITLE}` | Ticket title |
| `${TICKET_DESCRIPTION}` | Ticket description |
| `${PROJECT_ID}` | Project identifier |
| `${WORKFLOW}` | Workflow name (e.g., "feature") |
| `${USER_INSTRUCTIONS}` | User instructions from workflow run |
| `${CALLBACK_INSTRUCTIONS}` | Callback instructions when re-run via callback (see below) |
| `${PREVIOUS_DATA}` | Data from previous continued session |
| `${PARENT_SESSION}` | Parent orchestration session UUID |
| `${CHILD_SESSION}` | This agent's session UUID |
| `${MODEL_ID}` | Full model ID (e.g., "claude:opus") |
| `${MODEL}` | Model name (e.g., "opus") |

Findings from previous agents: `#{FINDINGS:agent-type}` or `#{FINDINGS:agent-type:key1,key2}`.

## Callback Mechanism

Callbacks allow a later-layer agent to request re-execution of an earlier layer when issues are found.

### How It Works

1. A later-layer agent (e.g., qa-verifier at layer 3) detects an issue
2. The agent saves callback instructions as a finding and calls `agent callback`
3. The orchestrator resets phases/sessions from the target layer forward and re-runs from there
4. The target agent (e.g., implementor at layer 2) receives callback instructions via `${CALLBACK_INSTRUCTIONS}`
5. After the callback target layer completes, `_callback` metadata is cleared

### Triggering a Callback (for verifier/reviewer agents)

```bash
# 1. Save what needs to be fixed as a finding
nrflow findings add <ticket> <agent-type> callback_instructions:"The auth middleware is not checking token expiry. Fix the validateToken function in middleware/auth.go." -w <workflow>

# 2. Trigger callback to an earlier layer
nrflow agent callback <ticket> <agent-type> -w <workflow> --level <N>
```

Where `--level <N>` is the layer number to jump back to (e.g., `--level 2` for the implementor layer).

### Receiving Callback Instructions (for implementor/target agents)

Add `${CALLBACK_INSTRUCTIONS}` to the agent's prompt template:

```markdown
## Instructions

${CALLBACK_INSTRUCTIONS}

## Your Task
...
```

- During normal execution: resolves to `"_No callback instructions_"`
- During callback re-run: resolves to formatted markdown with the issue details and source agent

### Limits

- Maximum 3 callbacks per workflow run
- The callback target layer and all layers between it and the calling layer are reset
