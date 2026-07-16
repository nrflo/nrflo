package consoleui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	base, token, project, session string
	http                          *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		base: strings.TrimRight(cfg.BaseURL, "/"), token: cfg.Token,
		project: cfg.Project, session: cfg.Session, http: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Create(ctx context.Context, engine, model, effort string) (string, error) {
	var response struct {
		SessionID string `json:"session_id"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/console/chats", map[string]string{
		"engine": engine, "model": model, "reasoning_effort": effort,
	}, &response)
	if err != nil {
		return "", err
	}
	c.session = response.SessionID
	return response.SessionID, nil
}

func (c *Client) Detail(ctx context.Context) (ChatDetail, error) {
	var result ChatDetail
	err := c.do(ctx, http.MethodGet, c.chatPath(""), nil, &result)
	return result, err
}

func (c *Client) Catalog(ctx context.Context) (Catalog, error) {
	var result Catalog
	err := c.do(ctx, http.MethodGet, "/api/v1/console/catalog", nil, &result)
	return result, err
}

func (c *Client) MessagesPage(ctx context.Context, limit, offset int) (MessagePage, error) {
	var result MessagePage
	path := fmt.Sprintf("%s?limit=%d&offset=%d", c.chatPath("messages"), limit, offset)
	err := c.do(ctx, http.MethodGet, path, nil, &result)
	return result, err
}

func (c *Client) TailMessages(ctx context.Context, limit int) (MessagePage, error) {
	page, err := c.MessagesPage(ctx, 1, 0)
	if err != nil || page.Total <= limit {
		if err != nil {
			return MessagePage{}, err
		}
		return c.MessagesPage(ctx, max(1, limit), 0)
	}
	return c.MessagesPage(ctx, limit, page.Total-limit)
}

func (c *Client) Send(ctx context.Context, text string) error {
	return c.do(ctx, http.MethodPost, c.chatPath("messages"), map[string]string{"text": text}, nil)
}

func (c *Client) Approve(ctx context.Context, id, decision string) error {
	return c.do(ctx, http.MethodPost, c.chatPath("approvals/"+url.PathEscape(id)), map[string]string{"decision": decision}, nil)
}

func (c *Client) Interrupt(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, c.chatPath("interrupt"), nil, nil)
}

func (c *Client) Close(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, c.chatPath("close"), nil, nil)
}

func (c *Client) chatPath(suffix string) string {
	p := "/api/v1/console/chats/" + url.PathEscape(c.session)
	if suffix != "" {
		p += "/" + suffix
	}
	return p
}

func (c *Client) do(ctx context.Context, method, requestPath string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+requestPath, reader)
	if err != nil {
		return err
	}
	c.setHeaders(req.Header)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("nrflo %s %s: %s", method, requestPath, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (c *Client) setHeaders(header http.Header) {
	header.Set("Authorization", "Bearer "+c.token)
	header.Set("X-Project", c.project)
}

func (c *Client) dialEvents(ctx context.Context) (*websocket.Conn, error) {
	u, err := url.Parse(c.base)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = "/api/v1/ws"
	header := http.Header{}
	c.setHeaders(header)
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, u.String(), header)
	if err != nil {
		return nil, err
	}
	subscribe := map[string]string{"action": "subscribe_session", "session_id": c.session}
	if err := conn.WriteJSON(subscribe); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}
