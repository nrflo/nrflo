package spawner

import (
	"encoding/json"
	"fmt"
	"strings"
)

const codexQuestionMethod = "item/tool/requestUserInput"
const CodexQuestionTool = "RequestUserInput"

type codexQuestionParams struct {
	ItemID    string `json:"itemId"`
	Questions []struct {
		ID       string `json:"id"`
		IsSecret bool   `json:"isSecret"`
	} `json:"questions"`
}

type codexQuestionResponse struct {
	Answers map[string]struct {
		Answers []string `json:"answers"`
	} `json:"answers"`
}

func (e *codexEngine) onQuestionRequest(env rpcEnvelope) {
	var p codexQuestionParams
	if json.Unmarshal(env.Params, &p) != nil || len(p.Questions) == 0 {
		_ = e.client.replyError(*env.ID, -32602, "console engine: invalid question request")
		return
	}
	seen := make(map[string]bool, len(p.Questions))
	for _, q := range p.Questions {
		if q.ID == "" || seen[q.ID] {
			_ = e.client.replyError(*env.ID, -32602, "console engine: invalid question id")
			return
		}
		seen[q.ID] = true
	}
	id := string(*env.ID)
	e.approvals.register(id, pendingApproval{rawID: *env.ID, method: env.Method, params: env.Params})
	e.emit(EngineEvent{
		Type: EventApprovalRequest, SessionID: e.spec.SessionID, ItemID: p.ItemID,
		Approval: &ApprovalRequest{ID: id, Kind: env.Method, Tool: CodexQuestionTool, Raw: env.Params},
	})
}

func (e *codexEngine) AnswerQuestion(id, answer string) error {
	pa, ok := e.approvals.peek(id)
	if !ok {
		return fmt.Errorf("console engine: no pending approval %q", id)
	}
	if pa.method != codexQuestionMethod {
		return fmt.Errorf("console engine: approval %q is not a question", id)
	}
	var request codexQuestionParams
	var response codexQuestionResponse
	if json.Unmarshal(pa.params, &request) != nil || json.Unmarshal([]byte(answer), &response) != nil {
		return fmt.Errorf("console engine: invalid question answer")
	}
	if len(response.Answers) != len(request.Questions) {
		return fmt.Errorf("console engine: answer required for every question")
	}
	labels := make([]string, 0, len(request.Questions))
	for _, q := range request.Questions {
		entry, ok := response.Answers[q.ID]
		if !ok || len(entry.Answers) == 0 {
			return fmt.Errorf("console engine: answer required for question %q", q.ID)
		}
		for _, choice := range entry.Answers {
			if strings.TrimSpace(choice) == "" {
				return fmt.Errorf("console engine: empty answer for question %q", q.ID)
			}
		}
		label := strings.Join(entry.Answers, ", ")
		if q.IsSecret {
			label = "[redacted]"
		}
		labels = append(labels, label)
	}
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	if client == nil {
		return fmt.Errorf("console engine: not started")
	}
	if err := client.reply(pa.rawID, response); err != nil {
		return err
	}
	e.approvals.drop(id)
	e.emit(EngineEvent{Type: EventApprovalResolved, SessionID: e.spec.SessionID, ApprovalID: id, Decision: ApprovalAnswer, Text: strings.Join(labels, "; ")})
	return nil
}
