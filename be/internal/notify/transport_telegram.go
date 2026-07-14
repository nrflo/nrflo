package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"be/internal/logger"
)

// TelegramBaseURL is the Telegram Bot API base. Overridable in tests.
var TelegramBaseURL = "https://api.telegram.org"

// errEntityParse marks a Telegram 400 caused by a malformed MarkdownV2
// entity (e.g. an unbalanced * or ` in a user-editable template), as
// distinct from other retryable 400s (chat not found, flood control, ...).
var errEntityParse = errors.New("telegram: entity parse error")

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

	// Telegram caps a message at telegramMaxLen; longer bodies are sent as
	// multiple sequential messages rather than truncated.
	for _, chunk := range splitTelegram(n.Body) {
		if err := t.sendChunk(n, botToken, chatID, chunk); err != nil {
			return err
		}
	}
	return nil
}

// sendChunk attempts to deliver chunk formatted as MarkdownV2. If Telegram
// rejects it as a malformed entity, it retries once as plaintext so a
// malformed *template* still delivers instead of being lost.
func (t *telegramTransport) sendChunk(n *Notification, botToken, chatID, chunk string) error {
	err := t.sendOne(botToken, chatID, chunk, "MarkdownV2")
	if err == nil {
		return nil
	}
	if !errors.Is(err, errEntityParse) {
		return err
	}
	logger.Warn(context.Background(), "telegram: entity parse failed, retrying as plaintext",
		"delivery_id", n.DeliveryID, "channel_id", n.ChannelID, "error", err)
	if err2 := t.sendOne(botToken, chatID, stripMarkdownV2(chunk), ""); err2 != nil {
		// Keep the original description: without it the recorded error only
		// shows the fallback's cause (429, network, ...) and the malformed
		// body is invisible outside the server log.
		return fmt.Errorf("telegram: plaintext fallback after %v also failed: %w", err, err2)
	}
	return nil
}

func (t *telegramTransport) sendOne(botToken, chatID, text, parseMode string) error {
	payload, err := json.Marshal(struct {
		ChatID    string `json:"chat_id"`
		Text      string `json:"text"`
		ParseMode string `json:"parse_mode,omitempty"`
	}{ChatID: chatID, Text: text, ParseMode: parseMode})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", TelegramBaseURL, botToken)
	resp, err := sharedClient.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("telegram: http error: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	jsonErr := json.Unmarshal(raw, &result)
	if jsonErr != nil {
		if resp.StatusCode == 200 {
			return nil
		}
		snippet := raw
		if len(snippet) > 256 {
			snippet = snippet[:256]
		}
		return fmt.Errorf("telegram: status %d, body: %s", resp.StatusCode, string(snippet))
	}
	if !result.OK {
		if isEntityParseError(result.Description) {
			return fmt.Errorf("telegram: %s: %w", result.Description, errEntityParse)
		}
		return fmt.Errorf("telegram: %s", result.Description)
	}
	return nil
}
