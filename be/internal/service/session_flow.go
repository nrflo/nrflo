package service

import (
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
			StartedAt:          sess.StartedAt.String,
			EndedAt:            sess.EndedAt.String,
			Depth:              cur.depth,
		}
		if sess.ContextLeft.Valid {
			pct := int(sess.ContextLeft.Int64)
			node.ContextLeft = &pct
		}
		nodes = append(nodes, node)

		children := repos.children(cur.sessionID, sess.WorkflowInstanceID)
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
// runs it launched, and console siblings opened from it.
func (r sessionFlowRepos) children(sessionID, wfiID string) []types.SessionFlowEdge {
	var edges []types.SessionFlowEdge

	if delegations, err := r.delegations.ListByCallerSession(sessionID); err == nil {
		for _, d := range delegations {
			for _, workerID := range d.WorkerSessionIDs {
				if workerID == "" {
					continue
				}
				edges = append(edges, types.SessionFlowEdge{FromSessionID: sessionID, ToSessionID: workerID, Kind: types.SessionFlowEdgeDelegate})
			}
		}
	}

	if consults, err := r.consults.ListByCallerSession(sessionID); err == nil {
		for _, c := range consults {
			if c.ChildSessionID == "" {
				continue
			}
			edges = append(edges, types.SessionFlowEdge{FromSessionID: sessionID, ToSessionID: c.ChildSessionID, Kind: types.SessionFlowEdgeConsult})
		}
	}

	if wfiID != "" {
		if children, err := r.wfi.ListByParentInstance(wfiID); err == nil {
			for _, child := range children {
				if child.ParentSession.Valid && child.ParentSession.String != sessionID {
					continue
				}
				r.appendInstanceEntryEdge(&edges, sessionID, child.ID, types.SessionFlowEdgeSubworkflow)
			}
		}
	}

	if origins, err := r.wfi.ListByOriginSession(sessionID); err == nil {
		for _, wi := range origins {
			r.appendInstanceEntryEdge(&edges, sessionID, wi.ID, types.SessionFlowEdgeOrigin)
		}
	}

	if siblings, err := r.sessions.ListSiblingsByOrigin(sessionID); err == nil {
		for _, sib := range siblings {
			edges = append(edges, types.SessionFlowEdge{FromSessionID: sessionID, ToSessionID: sib.ID, Kind: types.SessionFlowEdgeSibling})
		}
	}

	return edges
}

// appendInstanceEntryEdge resolves childInstanceID's earliest session as the
// flow-graph entry node for that instance (the graph is session-rooted, not
// instance-rooted) and appends one edge to it, when a session exists yet.
func (r sessionFlowRepos) appendInstanceEntryEdge(edges *[]types.SessionFlowEdge, fromSessionID, childInstanceID, kind string) {
	entry, err := r.sessions.FirstSessionForInstance(childInstanceID)
	if err != nil || entry == nil {
		return
	}
	*edges = append(*edges, types.SessionFlowEdge{FromSessionID: fromSessionID, ToSessionID: entry.ID, Kind: kind})
}
