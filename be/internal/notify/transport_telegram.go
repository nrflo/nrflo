package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// TelegramBaseURL is the Telegram Bot API base. Overridable in tests.
var TelegramBaseURL = "https://api.telegram.org"

type telegramTransport struct{}

func init() { Register(&telegramTransport{}) }

func (t *telegramTransport) Kind() string { return "telegram" }

func (t *telegramTransport) Send(n *Notification) error {
	botToken, _ := n.Config["bot_token"].(string)
	chatID, _ := n.Config["chat_id"].(string)
	if botToken == "" {
		return fmt.Errorf("telegram: bot_token not configured")
	}
	if chatID == "" {
		return fmt.Errorf("telegram: chat_id not configured")
	}

	payload, err := json.Marshal(map[string]string{
		"chat_id":    chatID,
		"text":       n.Body,
		"parse_mode": "MarkdownV2",
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", TelegramBaseURL, botToken)
	resp, err := sharedClient.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("telegram: http error: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if jsonErr := json.Unmarshal(raw, &result); jsonErr != nil {
		return fmt.Errorf("telegram: status %d, body: %s", resp.StatusCode, string(raw))
	}
	if !result.OK {
		return fmt.Errorf("telegram: %s", result.Description)
	}
	return nil
}
