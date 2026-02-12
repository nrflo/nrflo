package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// HTTPClient is a lightweight HTTP client for the nrworkflow API
type HTTPClient struct {
	BaseURL   string
	ProjectID string
	client    *http.Client
}

// NewHTTPClient creates a new HTTP client
func NewHTTPClient(baseURL, projectID string) *HTTPClient {
	return &HTTPClient{
		BaseURL:   baseURL,
		ProjectID: projectID,
		client:    &http.Client{},
	}
}

// Get performs a GET request
func (c *HTTPClient) Get(path string, result interface{}) error {
	return c.do("GET", path, nil, result)
}

// Post performs a POST request
func (c *HTTPClient) Post(path string, body, result interface{}) error {
	return c.do("POST", path, body, result)
}

// Patch performs a PATCH request
func (c *HTTPClient) Patch(path string, body, result interface{}) error {
	return c.do("PATCH", path, body, result)
}

// Delete performs a DELETE request
func (c *HTTPClient) Delete(path string, body, result interface{}) error {
	return c.do("DELETE", path, body, result)
}

func (c *HTTPClient) do(method, path string, body, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.ProjectID != "" {
		req.Header.Set("X-Project", c.ProjectID)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("%s", errResp.Error)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}

	return nil
}
