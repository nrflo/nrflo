package service

import (
	"strings"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/repo"
	"be/internal/types"
)

// sessionFlowMaxDepth/sessionFlowMaxNodes cap BuildSessionFlow's BFS —
// mirrors trace's marker_limit truncation guard (ParseTraceOptions) so a
// pathological delegate/consult/subworkflow cycle or a very wide fanout can
// never make one request unbounded.
const (
	sessionFlowMaxDepth = 8
	sessionFlowMaxNodes = 500
)

// sessionFlowRepos bundles the read paths BuildSessionFlow walks — one
// struct instead of five positional params threaded through the recursion.
type sessionFlowRepos struct {
	sessions    *repo.AgentSessionRepo
	delegations *repo.DelegationRepo
	consults    *repo.ConsultRepo
	wfi         *repo.WorkflowInstanceRepo
}

// BuildSessionFlow assembles the read-time transitive closure over
// delegations (caller->workers), consults (caller->child), sub-workflow
// children (workflow_instances.parent_instance_id + parent_session),
// origin-attributed runs (workflow_instances.origin_session_id — hidden
// delegate-host and console-initiated runs), and console sibling chats
// (agent_sessions.sibling_origin_session_id), rooted at rootSessionID. No
// state is written; mirrors service.BuildTrace's read-model role, generalized
// across workflow instances and session kinds instead of one instance.
func BuildSessionFlow(pool *db.Pool, clk clock.Clock, rootSessionID string) (*types.SessionFlowResponse, error) {
	repos := sessionFlowRepos{
		sessions:    repo.NewAgentSessionRepo(pool, clk),
		delegations: repo.NewDelegationRepo(pool, clk),
		consults:    repo.NewConsultRepo(pool, clk),
		wfi:         repo.NewWorkflowInstanceRepo(pool, clk),
	}

	visited := map[string]bool{}
	nodes := []types.SessionFlowNode{}
	edges := []types.SessionFlowEdge{}
	titles := map[string]string{}
	truncated := false

	type queued struct {
		sessionID string
		depth     int
	}
	queue := []queued{{sessionID: rootSessionID, depth: 0}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur.sessionID] {
			continue
		}
		if len(nodes) >= sessionFlowMaxNodes || cur.depth > sessionFlowMaxDepth {
			truncated = true
			continue
		}
		visited[cur.sessionID] = true

		sess, err := repos.sessions.Get(cur.sessionID)
		if err != nil {
			// A referenced session id with no row (purged, bad data) still
			// gets a bare node so edges pointing at it aren't dangling.
			nodes = append(nodes, types.SessionFlowNode{SessionID: cur.sessionID, Depth: cur.depth})
			continue
		}
		node := types.SessionFlowNode{
			SessionID:          sess.ID,
			Kind:               sess.Kind,
			AgentType:          sess.AgentType,
			Status:             string(sess.Status),
			Result:             sess.Result.String,
			WorkflowInstanceID: sess.WorkflowInstanceID,
			ModelID:            sess.ModelID.String,
			Title:              titles[sess.ID],
			StartedAt:          sess.StartedAt.String,
			EndedAt:            sess.EndedAt.String,
			Depth:              cur.depth,
		}
		if sess.ContextLeft.Valid {
			pct := int(sess.ContextLeft.Int64)
			node.ContextLeft = &pct
		}
		nodes = append(nodes, node)

		children := repos.children(cur.sessionID, sess.WorkflowInstanceID, titles)
		for _, edge := range children {
			edges = append(edges, edge)
			queue = append(queue, queued{sessionID: edge.ToSessionID, depth: cur.depth + 1})
		}
	}

	return &types.SessionFlowResponse{
		RootSessionID: rootSessionID,
		Nodes:         nodes,
		Edges:         edges,
		Truncated:     truncated,
	}, nil
}

// children resolves one session's outgoing flow edges: delegate workers,
// consult children, sub-workflow children it parented, origin-attributed
// runs it launched, and console siblings opened from it. Edges are deduped by
// target session — a delegate worker also reachable through its hidden
// _delegate_host instance's origin edge appears once, as the (earlier,
// more specific) delegate edge. titles collects per-target task labels
// (delegation brief / consult question / launched workflow name).
func (r sessionFlowRepos) children(sessionID, wfiID string, titles map[string]string) []types.SessionFlowEdge {
	var edges []types.SessionFlowEdge
	linked := map[string]bool{}
	add := func(toSessionID, kind string) {
		if toSessionID == "" || linked[toSessionID] {
			return
		}
		linked[toSessionID] = true
		edges = append(edges, types.SessionFlowEdge{FromSessionID: sessionID, ToSessionID: toSessionID, Kind: kind})
	}

	if delegations, err := r.delegations.ListByCallerSession(sessionID); err == nil {
		for _, d := range delegations {
			for _, workerID := range d.WorkerSessionIDs {
				setTitle(titles, workerID, d.Brief)
				add(workerID, types.SessionFlowEdgeDelegate)
			}
		}
	}

	if consults, err := r.consults.ListByCallerSession(sessionID); err == nil {
		for _, c := range consults {
			setTitle(titles, c.ChildSessionID, c.Question)
			add(c.ChildSessionID, types.SessionFlowEdgeConsult)
		}
	}

	if wfiID != "" {
		if children, err := r.wfi.ListByParentInstance(wfiID); err == nil {
			for _, child := range children {
				if child.ParentSession.Valid && child.ParentSession.String != sessionID {
					continue
				}
				r.addInstanceEntryEdge(add, titles, child.ID, child.WorkflowID, types.SessionFlowEdgeSubworkflow)
			}
		}
	}

	if origins, err := r.wfi.ListByOriginSession(sessionID); err == nil {
		for _, wi := range origins {
			// Every session of an origin instance, not just the earliest: a
			// reusable host instance (e.g. _refinery_fold) accrues one one-off
			// session per run, and the entry-only view would freeze the graph
			// on the first forever.
			sessions, err := r.sessions.SessionsForInstance(wi.ID)
			if err != nil {
				continue
			}
			for _, s := range sessions {
				setTitle(titles, s.ID, workflowTitle(wi.WorkflowID))
				add(s.ID, types.SessionFlowEdgeOrigin)
			}
		}
	}

	if siblings, err := r.sessions.ListSiblingsByOrigin(sessionID); err == nil {
		for _, sib := range siblings {
			add(sib.ID, types.SessionFlowEdgeSibling)
		}
	}

	return edges
}

// addInstanceEntryEdge resolves childInstanceID's earliest session as the
// flow-graph entry node for that instance (the graph is session-rooted, not
// instance-rooted) and appends one edge to it, when a session exists yet.
func (r sessionFlowRepos) addInstanceEntryEdge(add func(toSessionID, kind string), titles map[string]string, childInstanceID, workflowID, kind string) {
	entry, err := r.sessions.FirstSessionForInstance(childInstanceID)
	if err != nil || entry == nil {
		return
	}
	setTitle(titles, entry.ID, workflowTitle(workflowID))
	add(entry.ID, kind)
}

// flowTitleCap bounds a node title to one compact line.
const flowTitleCap = 100

// setTitle records text's first line (capped) as sessionID's task title,
// first writer wins.
func setTitle(titles map[string]string, sessionID, text string) {
	if sessionID == "" || text == "" || titles[sessionID] != "" {
		return
	}
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		text = text[:i]
	}
	text = strings.TrimSpace(text)
	if r := []rune(text); len(r) > flowTitleCap {
		text = string(r[:flowTitleCap-1]) + "…"
	}
	if text != "" {
		titles[sessionID] = text
	}
}

// workflowTitle labels an instance-entry node with its workflow name; hidden
// host workflows ("_"-prefixed) carry no user-meaningful name.
func workflowTitle(workflowID string) string {
	if strings.HasPrefix(workflowID, "_") {
		return ""
	}
	return workflowID
}
