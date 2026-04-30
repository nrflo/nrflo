package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type slackTransport struct{}

func init() { Register(&slackTransport{}) }

func (t *slackTransport) Kind() string { return "slack" }

func (t *slackTransport) Send(n *Notification) error {
	webhookURL, _ := n.Config["webhook_url"].(string)
	if webhookURL == "" {
		return fmt.Errorf("slack: webhook_url not configured")
	}

	body, err := json.Marshal(map[string]string{"text": n.Body})
	if err != nil {
		return err
	}

	resp, err := sharedClient.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack: http error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("slack: unexpected status %d: %s", resp.StatusCode, string(snippet))
	}
	return nil
}
