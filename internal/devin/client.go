// Package devin provides a client for the Devin AI API.
// Devin is an API-based coding agent that runs remotely, unlike CLI-based agents
// (claude, codex, etc.) that run locally in tmux sessions.
package devin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// DefaultBaseURL is the default Devin API base URL.
	DefaultBaseURL = "https://api.devin.ai/v1"

	// DefaultTimeout is the default HTTP client timeout.
	DefaultTimeout = 30 * time.Second
)

// Common errors.
var (
	ErrNoAPIKey       = errors.New("devin: API key is required")
	ErrSessionNotFound = errors.New("devin: session not found")
	ErrUnauthorized   = errors.New("devin: unauthorized - check API key")
	ErrTimeout        = errors.New("devin: timeout waiting for session completion")
)

// SessionStatus represents the status of a Devin session.
type SessionStatus string

// Session status constants.
const (
	SessionStatusRunning  SessionStatus = "running"
	SessionStatusBlocked  SessionStatus = "blocked"
	SessionStatusFinished SessionStatus = "finished"
	SessionStatusStopped  SessionStatus = "stopped"
	SessionStatusFailed   SessionStatus = "failed"
)

// IsTerminal returns true if the session status indicates the session has ended.
func (s SessionStatus) IsTerminal() bool {
	switch s {
	case SessionStatusFinished, SessionStatusStopped, SessionStatusFailed:
		return true
	default:
		return false
	}
}

// PullRequest contains information about a PR created by Devin.
type PullRequest struct {
	URL string `json:"url"`
}

// Session represents a Devin session.
type Session struct {
	SessionID          string       `json:"session_id"`
	Status             SessionStatus `json:"status"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
	PlaybookID         string       `json:"playbook_id,omitempty"`
	PullRequest        *PullRequest `json:"pull_request,omitempty"`
	RequestingUserEmail string      `json:"requesting_user_email,omitempty"`
}

// CreateSessionRequest contains the parameters for creating a new session.
type CreateSessionRequest struct {
	Prompt       string   `json:"prompt"`
	Idempotent   bool     `json:"idempotent,omitempty"`
	KnowledgeIDs []string `json:"knowledge_ids,omitempty"`
	MaxACULimit  int      `json:"max_acu_limit,omitempty"`
	PlaybookID   string   `json:"playbook_id,omitempty"`
	SecretIDs    []string `json:"secret_ids,omitempty"`
}

// SendMessageRequest contains the parameters for sending a message to a session.
type SendMessageRequest struct {
	Message string `json:"message"`
}

// ListSessionsResponse contains the response from listing sessions.
type ListSessionsResponse struct {
	Sessions []Session `json:"sessions"`
}

// Client is a client for the Devin API.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Devin API client.
func NewClient(apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, ErrNoAPIKey
	}
	return &Client{
		apiKey:  apiKey,
		baseURL: DefaultBaseURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}, nil
}

// WithBaseURL sets a custom base URL for the client.
// This is useful for testing with a mock server.
func (c *Client) WithBaseURL(url string) *Client {
	c.baseURL = url
	return c
}

// WithHTTPClient sets a custom HTTP client.
func (c *Client) WithHTTPClient(client *http.Client) *Client {
	c.httpClient = client
	return c
}

// doRequest performs an HTTP request with authentication.
func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	return c.httpClient.Do(req)
}

// handleResponse checks the response status and returns an appropriate error.
func handleResponse(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		return nil
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrSessionNotFound
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("devin API error (status %d): %s", resp.StatusCode, string(body))
	}
}

// CreateSession creates a new Devin session.
func (c *Client) CreateSession(req CreateSessionRequest) (*Session, error) {
	resp, err := c.doRequest(http.MethodPost, "/sessions", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := handleResponse(resp); err != nil {
		return nil, err
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &session, nil
}

// GetSession retrieves information about an existing session.
func (c *Client) GetSession(sessionID string) (*Session, error) {
	resp, err := c.doRequest(http.MethodGet, "/sessions/"+sessionID, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := handleResponse(resp); err != nil {
		return nil, err
	}

	var session Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &session, nil
}

// SendMessage sends a message to an existing session.
func (c *Client) SendMessage(sessionID, message string) error {
	req := SendMessageRequest{Message: message}
	resp, err := c.doRequest(http.MethodPost, "/sessions/"+sessionID+"/message", req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return handleResponse(resp)
}

// TerminateSession terminates an existing session.
func (c *Client) TerminateSession(sessionID string) error {
	resp, err := c.doRequest(http.MethodDelete, "/sessions/"+sessionID, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return handleResponse(resp)
}

// ListSessions lists all sessions.
func (c *Client) ListSessions() ([]Session, error) {
	resp, err := c.doRequest(http.MethodGet, "/sessions", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := handleResponse(resp); err != nil {
		return nil, err
	}

	var listResp ListSessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return listResp.Sessions, nil
}

// WaitForCompletion polls the session until it reaches a terminal state.
// It returns the final session state or an error if the timeout is reached.
func (c *Client) WaitForCompletion(sessionID string, pollInterval, timeout time.Duration) (*Session, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		session, err := c.GetSession(sessionID)
		if err != nil {
			return nil, err
		}

		if session.Status.IsTerminal() {
			return session, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, ErrTimeout
}
