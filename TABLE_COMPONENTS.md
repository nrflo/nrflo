# UI Table Components & Styling Analysis

## Summary

There is **NO shadcn/ui Table component** in the nrworkflow UI codebase. Instead, all data tables use a **custom flex-based card layout** approach that was implemented in commit `81f7671` ("Restyle all data tables to flex-based card layout matching ChainDetailPage").

## 1. No shadcn Table Component

**Finding:** `ui/src/components/ui/` does not contain a `table.tsx` or any shadcn-based table component.

The project moved away from HTML `<table>` elements (which used `<thead>`, `<tbody>`, `<tr>`, `<td>`) to a flex-based approach for better responsiveness and consistency.

## 2. Table-Rendering Components

All custom tables in the codebase follow the same **flex-based card layout pattern**:

### Core Tables:

1. **TicketTable.tsx** (`ui/src/components/tickets/TicketTable.tsx`)
   - Lists all tickets with columns: Type, ID, Title, Status, Priority, Created By, Updated, Progress
   - Sortable columns with chevron indicators
   - Clickable rows navigate to ticket detail

2. **CompletedAgentsTable.tsx** (`ui/src/components/workflow/CompletedAgentsTable.tsx`)
   - Lists completed agents in paginated table (20 items/page)
   - Columns: Agent, Phase, Model, Result, Duration, Completed At
   - Clickable rows select agent for detail view

3. **WorkflowInstanceTable.tsx** (`ui/src/pages/WorkflowInstanceTable.tsx`)
   - Lists workflow instances (project-scoped workflows) (10 items/page)
   - Columns: Workflow, Instance, Status, Duration, Completed At, Delete
   - Used in ProjectWorkflowsPage Failed/Completed tabs

4. **AgentLogDetail.tsx - Message Table** (`ui/src/components/workflow/AgentLogDetail.tsx`)
   - Displays agent messages in a table format
   - Columns: Time, Tool, Message
   - Rendered as flex container with text overflow handling

5. **ChainDetailPage - Item Rows** (`ui/src/pages/ChainDetailPage.tsx`)
   - Chain execution items listed as flex rows
   - Displays position, ticket ID, title, status, duration, tokens

## 3. Flex-Based Card Layout Pattern

All tables now use this consistent structure:

```
┌─ Outer Container ──────────────────────────────────────┐
│  <div className="border border-border rounded-lg">    │
│                                                         │
│  ┌─ Header (bg-muted/30) ─────────────────────────────┐│
│  │ <div className="px-4 py-2 border-b border-border"> ││
│  │   <div className="flex items-center gap-4          ││
│  │         text-xs font-medium text-muted-foreground  ││
│  │         uppercase tracking-wider">                 ││
│  │     <span className="w-10 shrink-0">Type</span>    ││
│  │     <span className="w-32 shrink-0">ID</span>      ││
│  │     <span className="flex-1 min-w-0">Title</span>  ││
│  │     ...more columns...                             ││
│  │   </div>                                            ││
│  │ </div>                                              ││
│  └─────────────────────────────────────────────────────┘│
│                                                         │
│  ┌─ Row 1 ────────────────────────────────────────────┐│
│  │ <div className="flex items-center gap-4            ││
│  │       px-4 py-3 border-b border-border             ││
│  │       last:border-b-0 hover:bg-muted/50            ││
│  │       cursor-pointer transition-colors">           ││
│  │   <span className="w-10 shrink-0">🐛</span>        ││
│  │   <span className="w-32 shrink-0">TICKET-123</span>││
│  │   <span className="flex-1 min-w-0">Title...</span> ││
│  │   ...more columns...                               ││
│  │ </div>                                              ││
│  └─────────────────────────────────────────────────────┘│
│                                                         │
│  ┌─ Row 2 ────────────────────────────────────────────┐│
│  │ ...similar structure...                            ││
│  └─────────────────────────────────────────────────────┘│
│                                                         │
└─────────────────────────────────────────────────────────┘
```

## 4. Table Styling - Comprehensive Breakdown

### Outer Container
```tsx
<div className="border border-border rounded-lg text-xs font-mono">
```
- `border border-border` — 1px border with CSS variable color
- `rounded-lg` — border-radius
- `text-xs` — Tailwind text size (12px base on most tables)
- `font-mono` — Monospace font (used for data display)

### Header Row
```tsx
<div className="px-4 py-2 border-b border-border bg-muted/30">
  <div className="flex items-center gap-4 text-xs font-medium text-muted-foreground uppercase tracking-wider">
    <span className="w-10 shrink-0">Type</span>
    <span className="w-32 shrink-0">ID</span>
    <span className="flex-1 min-w-0">Title</span>
    ...
  </div>
</div>
```

**Header styling:**
- `px-4 py-2` — Horizontal padding 1rem, vertical padding 0.5rem
- `border-b border-border` — Bottom border separator
- `bg-muted/30` — Subtle background (muted color at 30% opacity)

**Header text:**
- `text-xs` — Small font
- `font-medium` — Weight 500
- `text-muted-foreground` — Dimmed color for labels
- `uppercase tracking-wider` — All caps with letter spacing

**Column headers:**
- `gap-4` — Space between columns (flexbox gap)
- Fixed-width columns: `w-10`, `w-32`, `w-24`, `w-20`, `w-28`, etc.
  - Use `shrink-0` to prevent flex shrinking
- Flexible columns: `flex-1 min-w-0` 
  - `flex-1` takes remaining space
  - `min-w-0` allows text truncation inside flexbox

### Data Rows
```tsx
<div className="flex items-center gap-4 px-4 py-3 border-b border-border last:border-b-0 hover:bg-muted/50 cursor-pointer transition-colors">
  <span className="w-10 shrink-0">🐛</span>
  <span className="w-32 shrink-0">TICKET-123</span>
  <span className="flex-1 min-w-0 truncate">Ticket title text...</span>
  ...
</div>
```

**Row styling:**
- `flex items-center gap-4` — Horizontal flexbox, vertically centered, 1rem gap
- `px-4 py-3` — Padding (1rem horizontal, 0.75rem vertical)
- `border-b border-border` — Separator between rows
- `last:border-b-0` — Remove bottom border from last row
- `hover:bg-muted/50` — Subtle hover effect
- `cursor-pointer transition-colors` — Interactive feedback

**Cell styling:**
- Fixed-width: `w-10 shrink-0` — Exactly 2.5rem, no flex shrinking
- Flexible: `flex-1 min-w-0` — Takes remaining space, allows truncation
- Text overflow: `truncate` (for single-line) or `whitespace-pre-wrap break-words` (for multi-line)
- Secondary text: `text-muted-foreground` — Dimmed color

### Special Styling Examples

**Overflow handling in fixed columns:**
```tsx
<span className="w-28 shrink-0 text-muted-foreground truncate">
  {ticket.created_by || '-'}
</span>
```
- `truncate` — Single line with ellipsis
- `text-muted-foreground` — Secondary color

**Overflow in flexible columns:**
```tsx
<span className="flex-1 min-w-0">
  <span className="whitespace-pre-wrap break-words">
    {agent.message}
  </span>
</span>
```
- `whitespace-pre-wrap` — Preserve whitespace, wrap text
- `break-words` — Break long words

**Selected state:**
```tsx
<div className={cn(
  'flex items-center gap-4 px-4 py-3 border-b border-border last:border-b-0 hover:bg-muted/50 cursor-pointer transition-colors',
  isSelected && 'bg-primary/10'  // Selection highlight
)}>
```

**Status badges in cells:**
```tsx
<span className="w-16 shrink-0">
  <Badge
    variant={isFailed ? 'destructive' : 'success'}
    className="text-[10px] px-1 py-0"
  >
    {status}
  </Badge>
</span>
```
- Badges use variant styling from Badge component
- Compact size with `text-[10px] px-1 py-0`

**Special coloring (e.g., rate limit messages):**
```tsx
<div className={cn("flex items-start gap-4 px-4 py-1 border-b border-border last:border-b-0", 
  toolName === 'rate_limit' && "bg-orange-50 dark:bg-orange-950/20")}>
```
- Semantic color (orange for rate limits)
- Dark mode aware with `dark:` variant

## 5. Actual Code Snippets by Component

### TicketTable (Full Row Example)
```tsx
<div
  key={ticket.id}
  onClick={() => navigate(`/tickets/${encodeURIComponent(ticket.id)}`)}
  className="flex items-center gap-4 px-4 py-3 border-b border-border last:border-b-0 hover:bg-muted/50 cursor-pointer transition-colors"
  data-testid="table-row"
>
  <span className="w-10 shrink-0">
    <IssueTypeIcon type={ticket.issue_type} />
  </span>
  <span className="w-32 shrink-0 flex items-center gap-1">
    {ticket.id}
    {isBlocked && (
      <span title="Blocked">
        <Lock className="h-3 w-3 text-orange-500" />
      </span>
    )}
  </span>
  <span className="flex-1 min-w-0 truncate">{ticket.title}</span>
  <span className="w-24 shrink-0">
    <Badge className={cn('text-xs px-1 py-0', statusColor(ticket.status))}>
      {ticket.status.replace('_', ' ')}
    </Badge>
  </span>
  <span className="w-20 shrink-0">{priorityLabel(ticket.priority)}</span>
  <span className="w-28 shrink-0 text-muted-foreground truncate">
    {ticket.created_by || '-'}
  </span>
  <span className="w-24 shrink-0 text-muted-foreground">
    {formatRelativeTime(ticket.updated_at)}
  </span>
  <span className="w-16 shrink-0">
    {progress && progress.total_phases > 0 ? (
      <div className="flex items-center gap-1">
        <div className="flex-1 h-1.5 bg-muted rounded-full overflow-hidden w-12">
          <div
            className="h-full bg-primary rounded-full transition-all"
            style={{
              width: `${Math.round((progress.completed_phases / progress.total_phases) * 100)}%`,
            }}
          />
        </div>
        <span className="text-muted-foreground whitespace-nowrap">
          {progress.completed_phases}/{progress.total_phases}
        </span>
      </div>
    ) : null}
  </span>
</div>
```

### AgentLogDetail Message Table (Flex-based)
```tsx
<div className="border border-border rounded-lg text-xs font-mono" data-testid="message-table">
  <div className="px-4 py-2 border-b border-border bg-muted/30">
    <div className="flex items-start gap-4 text-xs font-medium text-muted-foreground uppercase tracking-wider">
      <span className="w-[90px] shrink-0">Time</span>
      <span className="w-[100px] shrink-0">Tool</span>
      <span className="flex-1 min-w-0">Message</span>
    </div>
  </div>
  {[...filteredMessages].reverse().map((msg, i) => {
    const { toolName, rest } = parseToolName(msg.content)
    return (
      <div key={i} 
        className={cn("flex items-start gap-4 px-4 py-1 border-b border-border last:border-b-0", 
          toolName === 'rate_limit' && "bg-orange-50 dark:bg-orange-950/20")}
      >
        <span className="w-[90px] shrink-0 text-muted-foreground whitespace-nowrap overflow-hidden text-ellipsis">
          {formatTime(msg.created_at)}
        </span>
        <span className="w-[100px] shrink-0 overflow-hidden">
          {toolName && <ToolBadge name={toolName} />}
        </span>
        <span className="flex-1 min-w-0 whitespace-pre-wrap break-words text-foreground/90">
          {rest}
        </span>
      </div>
    )
  })}
</div>
```

## 6. Migration from HTML Tables

**Old approach (commit 5299e44):**
```tsx
<table className="w-full text-sm font-mono border-collapse">
  <thead>
    <tr className="text-left text-muted-foreground border-b border-border">
      <th className="py-1.5 pr-3 font-medium cursor-pointer">Type</th>
      ...
    </tr>
  </thead>
  <tbody>
    <tr className="border-b border-border/50 hover:bg-muted/50">
      <td className="py-1.5 pr-3 w-10">...</td>
      ...
    </tr>
  </tbody>
</table>
```

**New approach (commit 81f7671 onwards):**
```tsx
<div className="border border-border rounded-lg text-sm font-mono">
  <div className="px-4 py-2 border-b border-border bg-muted/30">
    <div className="flex items-center gap-4 ...">
      <span className="w-10 shrink-0">Type</span>
      ...
    </div>
  </div>
  <div className="flex items-center gap-4 px-4 py-3 ...">
    <span className="w-10 shrink-0">...</span>
    ...
  </div>
</div>
```

**Rationale for change:**
- Better responsive control (easier to hide columns on mobile)
- Consistent with ChainDetailPage pattern
- Simpler CSS architecture (flexbox vs table layout algorithm)
- Better control over cell alignment and overflow

## 7. Key CSS Patterns to Remember

| Pattern | Usage | Example |
|---------|-------|---------|
| `flex items-center gap-4` | Row container | `<div className="flex items-center gap-4 ...">`|
| `w-10 shrink-0` | Fixed-width column | Type icon (40px) |
| `w-32 shrink-0` | Fixed-width column | ID (128px) |
| `flex-1 min-w-0` | Flexible column | Title (remaining space) |
| `truncate` | Single-line overflow | `{ticket.title}` |
| `whitespace-pre-wrap break-words` | Multi-line overflow | Agent messages |
| `text-muted-foreground` | Secondary text | Column headers, dates |
| `bg-muted/30` | Header background | Table header row |
| `hover:bg-muted/50` | Row hover | Clickable rows |
| `border-b border-border last:border-b-0` | Row separators | Between rows, no bottom border |
| `text-xs uppercase tracking-wider` | Column labels | Header text |
| `font-mono` | Code/data display | Container class |

## 8. No Custom Table Component

There is **NO generic reusable table component** (like `<Table>` from shadcn). Each table is implemented inline in its page/component because:
- Tables have different columns and data structures
- Different interaction patterns (click behavior varies)
- Pagination handled differently per component
- Easier to maintain inline than create over-abstracted component

This is appropriate for this codebase given the variety of table types (ticket list, agent history, workflow instances, message log, chain items).

