---
name: prep
description: Plan a new or updated feature through iterative refinement, then create tickets via nrworkflow.
---

# Prep Skill

Plan and refine feature requirements through conversation, then generate tickets with nrworkflow.

## CRITICAL RULES

1. **ALL 8 PHASES ARE MANDATORY** - You MUST complete each phase in order. NO SHORTCUTS.
2. **NEVER create tickets without descriptions** - Every `nrworkflow ticket create` MUST use `-d "..."` flag
3. **VERIFY before proceeding** - Each phase has a checkpoint. Do not continue until verified.
4. **RESOLVE ALL QUESTIONS** - No questions allowed during /impl. Clarify everything here.

---

## Workflow Overview

```
Phase 1: Understand  ──✓──►  Phase 2: Explore  ──✓──►  Phase 3: Plan
                                                              │
                                                              ▼
Phase 4: Clarify  ◄────────────────────────────────────────  │
        │                                                     │
        ▼                                                     │
Phase 5: Iterate  ──✓──►  Phase 6: Epic Assessment  ──✓──►  │
                                                              │
                                                              ▼
                          Phase 8: Initialize  ◄──  Phase 7: Create Tickets
```

**You are currently at: Phase 1**

---

## Phase 1: Understand

**Goal**: Capture what the user wants to build.

Parse the user's request to identify:
- Is this a **new feature** or **update to existing**?
- What is the core functionality requested?
- Any constraints or preferences mentioned?

If the user provided arguments to the skill, use them as the feature description.
If no arguments, use AskUserQuestion:
```
"What feature would you like to plan?"
- Header: "Feature"
- Options: freeform text input expected
```

### Phase 1 Checkpoint ✓

Before proceeding to Phase 2, confirm you have:
- [ ] Clear description of what user wants
- [ ] Identified if new feature or update
- [ ] Noted any explicit constraints

**Output to user**: "I understand you want to [summary]. Let me explore the codebase to understand the context."

---

## Phase 2: Explore

**Goal**: Understand existing codebase context.

Spawn a codebase-explorer sub-agent:

```
subagent_type: codebase-explorer
prompt: |
  Explore the codebase to understand context for this feature:
  <feature description>

  Find:
  1. Related existing code (models, views, services)
  2. Patterns used in similar features
  3. Dependencies that might be affected
  4. Test patterns for this area

  Output a brief context summary (bullet points).
```

### Phase 2 Checkpoint ✓

Before proceeding to Phase 3, confirm you have:
- [ ] List of related existing files
- [ ] Identified patterns to follow
- [ ] Understood dependencies

**Output to user**: Show exploration results as bullet points.

---

## Phase 3: Plan

**Goal**: Create detailed feature plan for user review.

Based on exploration, create an initial feature plan with ALL of these sections:

1. **Summary**: One paragraph describing the feature
2. **User Stories**: 2-5 user stories in format "As a [user], I want [goal] so that [benefit]"
3. **Technical Approach**: How it fits into existing architecture
4. **Components**: List of components/files to create or modify
5. **Acceptance Criteria**: Testable criteria for completion
6. **Open Questions**: Things that need clarification (if any)
7. **Complexity Assessment**: Estimate if this is single-ticket or multi-ticket (epic)

### Phase 3 Checkpoint ✓

Before proceeding to Phase 4, confirm your plan has:
- [ ] Summary paragraph
- [ ] At least 2 user stories
- [ ] Technical approach section
- [ ] Specific files listed (with paths)
- [ ] At least 3 acceptance criteria
- [ ] Open questions identified (or "None")
- [ ] Complexity assessment

**Output to user**: Present the complete plan in a formatted block.

---

## Phase 4: Clarify

**Goal**: Resolve ALL open questions. No questions allowed during /impl.

Use AskUserQuestion to resolve:
- Technical approach alternatives
- Scope boundaries
- Implementation details
- Edge cases

**IMPORTANT**: Every question that might arise during implementation MUST be resolved here.

Structure questions efficiently:
- Group related questions when possible
- Provide reasonable default options
- Allow "Other" for custom input

Example:
```
questions:
  - question: "How should the data be persisted?"
    header: "Storage"
    options:
      - label: "SwiftData (Recommended)"
        description: "Consistent with existing models"
      - label: "UserDefaults"
        description: "For simple preferences"
      - label: "File system"
        description: "For large data or exports"
```

### Phase 4 Checkpoint ✓

Before proceeding to Phase 5, confirm:
- [ ] ALL open questions answered
- [ ] User has reviewed the plan
- [ ] No ambiguities remain
- [ ] Implementation approach is fully specified

---

## Phase 5: Iterate

**Goal**: Refine plan until user approves.

After each round of clarification:

1. Update the plan based on answers
2. Present the revised plan (show what changed)
3. Ask user using AskUserQuestion:

```
question: "Are you satisfied with this plan?"
header: "Plan Status"
options:
  - label: "Create tickets"
    description: "Plan is approved, create bd tickets now"
  - label: "Refine further"
    description: "I have more changes or questions"
  - label: "Start over"
    description: "Let's rethink this from scratch"
```

### Phase 5 Checkpoint ✓

Before proceeding to Phase 6, confirm:
- [ ] User explicitly selected "Create tickets"
- [ ] Final plan has all required sections
- [ ] No unresolved questions

**DO NOT proceed to Phase 6 without explicit user approval.**

---

## Phase 6: Epic Assessment

**Goal**: Determine if this is a single ticket or epic with sub-tickets.

### Single Ticket Criteria
- 1-2 files to modify
- 2-3 acceptance criteria
- Estimated work: 1-2 hours
- No complex dependencies

→ Skip to Phase 7 (single ticket)

### Epic Criteria
- 3+ files to modify
- 4+ acceptance criteria
- Multiple distinct components
- Clear phases of work

### Epic Mode Selection

If epic, use AskUserQuestion:

```
question: "This feature spans multiple components. How should we structure implementation?"
header: "Implementation Mode"
options:
  - label: "Single-shot (Recommended)"
    description: "Implement all sub-tickets together in one session"
  - label: "Separate"
    description: "Implement sub-tickets one at a time"
  - label: "Single ticket"
    description: "Combine into one larger ticket"
```

### Sub-Ticket Breakdown

If epic mode selected, break down into sub-tickets:

1. **Foundation**: Core models/infrastructure (always first)
2. **Integration**: Wire components together
3. **Polish**: Error handling, edge cases, UI refinements

Each sub-ticket must have:
- Clear scope (1-2 files max)
- 2-3 acceptance criteria
- No overlap with other sub-tickets
- Explicit dependencies

Store decision in plan:
```markdown
## Epic Mode
<single-shot|separate>

## Sub-tickets (in order)
1. <title> - <category: simple|full> - <files>
2. <title> - <category: simple|full> - <files>
...
```

### Phase 6 Checkpoint ✓

Before proceeding to Phase 7, confirm:
- [ ] Epic vs single ticket decision made
- [ ] If epic: implementation mode selected
- [ ] If epic: sub-tickets defined with no overlap
- [ ] Dependencies between sub-tickets documented

---

## Phase 7: Create Tickets

**Goal**: Convert approved plan into bd tickets with RICH DESCRIPTIONS.

### MANDATORY: Description Format

Every ticket description MUST include these sections:

```markdown
## Vision
Brief description of what this achieves and why it matters.

## Strategy
High-level approach to implementation.

## Phases (for features/epics)
1. **Phase 1**: Description
2. **Phase 2**: Description

## Files to Modify
- `path/to/file.swift` - What changes needed

## Files to Create
- `path/to/new-file.swift` - Purpose of new file

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2
```

### CORRECT Ticket Creation Syntax

**ALWAYS use nrworkflow ticket create with `-d` flag and `--init` for auto-initialization:**

```bash
# Epic
nrworkflow ticket create --type=epic --title="Feature Name" --priority=2 -d "## Vision
Enable X functionality for users.

## Strategy
Build on existing Y architecture...

## Implementation Mode
single-shot

## Sub-tickets (in order)
1. cclogs-xxx - Foundation (simple)
2. cclogs-yyy - Integration (full)

## Acceptance Criteria
- [ ] Users can do X
- [ ] Data persists correctly"

# Feature ticket (with auto-init for workflow state)
nrworkflow ticket create --type=feature --title="Component Name" --priority=2 --init -d "## Vision
Provide X capability.

## Strategy
Use existing Y pattern...

## Files to Create
- \`Sources/Core/Services/NewService.swift\` - Service implementation

## Files to Modify
- \`Sources/Core/Views/MainView.swift\` - Add button for new feature

## Acceptance Criteria
- [ ] Service handles X
- [ ] UI shows Y
- [ ] Tests cover Z"
```

### ANTI-PATTERNS - DO NOT DO THIS

```bash
# WRONG: Direct bd create (use nrworkflow ticket create instead)
bd create --title="Add feature" --type=feature --priority=2 -d "..."

# WRONG: No description (will fail validation)
nrworkflow ticket create --type=feature --title="Add feature" --priority=2 -d ""

# WRONG: Missing required sections (will fail validation)
nrworkflow ticket create --type=feature --title="Add feature" --priority=2 -d "Add the feature"

# WRONG: Heredoc doesn't work
nrworkflow ticket create --type=feature --title="Add feature" --priority=2 -d <<'EOF'
Description here
EOF
```

### Ticket Overlap Rules

**Before creating tickets, verify no overlap:**

1. **One file, one owner**: Each file modified by only ONE ticket
2. **No shared acceptance criteria**: Two tickets must NOT have overlapping criteria
3. **Clear boundaries**: Each ticket has distinct scope
4. **Atomic deliverables**: Each ticket delivers a complete, testable unit

### Dependency Rules

**Valid dependencies:**
- **Data flow**: B uses models/APIs created by A
- **Infrastructure**: B needs service/component from A
- **Sequential logic**: B's implementation only makes sense after A

**Invalid dependencies (do NOT create):**
- **Arbitrary ordering**: "A first because easier"
- **Shared files without data flow**: Both touch same file independently
- **Circular**: A → B → A

After creating tickets:
```bash
nrworkflow ticket dep add <child-id> <parent-id>  # child depends on parent
```

### Phase 7 Checkpoint ✓

After creating ALL tickets, verify:

```bash
# Check each ticket has a description
nrworkflow ticket show <ticket-id>
```

For EACH ticket, confirm the output shows:
- [ ] "Description:" section is present
- [ ] Description contains ## Vision
- [ ] Description contains ## Acceptance Criteria

---

## Phase 8: Verify Initialization

**Goal**: Verify tickets are ready for /impl.

If you used `--init` flag during ticket creation, workflow state is already initialized.

For any tickets created without `--init`, initialize manually:
```bash
nrworkflow init <ticket-id>
```

If epic mode was selected, set the mode on each sub-ticket:

```bash
nrworkflow set <ticket-id> epic_mode "single-shot"
# or
nrworkflow set <ticket-id> epic_mode "separate"
```

### Phase 8 Checkpoint ✓

Verify initialization:

```bash
nrworkflow status <ticket-id>
```

Should show:
- Initialized timestamp
- Current phase: investigation
- All phases: pending

---

## Final Output

After completing all phases, display:

1. **Summary table:**
   ```
   | ID         | Type    | Title                    | Status      |
   |------------|---------|--------------------------|-------------|
   | cclogs-xxx | epic    | Feature Name             | Initialized |
   | cclogs-yyy | feature | Component A              | Initialized |
   | cclogs-zzz | task    | Specific implementation  | Initialized |
   ```

2. **Dependency graph:**
   ```
   cclogs-xxx (epic)
   └── cclogs-yyy (feature)
       └── cclogs-zzz (task)
   ```

3. **Next steps:**
   ```bash
   nrworkflow ticket status  # Dashboard: pending + completed
   nrworkflow ticket ready   # Show unblocked tickets
   /impl <ticket>   # Start implementation
   ```

---

## Tips

- Keep tickets focused and atomic (1-2 day work max)
- Prefer more small tickets over fewer large ones
- Always include acceptance criteria with checkboxes
- Set realistic priorities (P2 for normal features)
- Use dependencies to show implementation order
- When in doubt, ask for clarification rather than assuming
- **All questions must be resolved here** - /impl runs without interruption
