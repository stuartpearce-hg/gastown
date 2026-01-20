package devin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		apiKey  string
		wantErr bool
	}{
		{
			name:    "valid API key",
			apiKey:  "test-api-key",
			wantErr: false,
		},
		{
			name:    "empty API key",
			apiKey:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.apiKey)
			if tt.wantErr {
				if err == nil {
					t.Error("NewClient() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("NewClient() unexpected error: %v", err)
				return
			}
			if client == nil {
				t.Error("NewClient() returned nil client")
			}
		})
	}
}

func TestClientWithBaseURL(t *testing.T) {
	t.Parallel()

	client, err := NewClient("test-key")
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}

	customURL := "https://custom.api.devin.ai/v1"
	client = client.WithBaseURL(customURL)

	if client.baseURL != customURL {
		t.Errorf("WithBaseURL() baseURL = %v, want %v", client.baseURL, customURL)
	}
}

func TestCreateSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		request        CreateSessionRequest
		serverResponse *Session
		serverStatus   int
		wantErr        bool
	}{
		{
			name: "successful session creation",
			request: CreateSessionRequest{
				Prompt: "Fix the bug in main.go",
			},
			serverResponse: &Session{
				SessionID: "session-123",
				Status:    SessionStatusRunning,
				CreatedAt: time.Now(),
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name: "session with playbook",
			request: CreateSessionRequest{
				Prompt:     "Implement feature X",
				PlaybookID: "playbook-456",
			},
			serverResponse: &Session{
				SessionID:  "session-456",
				Status:     SessionStatusRunning,
				PlaybookID: "playbook-456",
				CreatedAt:  time.Now(),
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name: "server error",
			request: CreateSessionRequest{
				Prompt: "Test prompt",
			},
			serverStatus: http.StatusInternalServerError,
			wantErr:      true,
		},
		{
			name: "unauthorized",
			request: CreateSessionRequest{
				Prompt: "Test prompt",
			},
			serverStatus: http.StatusUnauthorized,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				if r.URL.Path != "/sessions" {
					t.Errorf("expected /sessions, got %s", r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverResponse != nil {
					if err := json.NewEncoder(w).Encode(tt.serverResponse); err != nil {
						t.Errorf("failed to encode response: %v", err)
					}
				}
			}))
			defer server.Close()

			client, _ := NewClient("test-key")
			client = client.WithBaseURL(server.URL)

			session, err := client.CreateSession(tt.request)
			if tt.wantErr {
				if err == nil {
					t.Error("CreateSession() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("CreateSession() unexpected error: %v", err)
				return
			}
			if session.SessionID != tt.serverResponse.SessionID {
				t.Errorf("CreateSession() SessionID = %v, want %v", session.SessionID, tt.serverResponse.SessionID)
			}
		})
	}
}

func TestGetSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		sessionID      string
		serverResponse *Session
		serverStatus   int
		wantErr        bool
	}{
		{
			name:      "get running session",
			sessionID: "session-123",
			serverResponse: &Session{
				SessionID: "session-123",
				Status:    SessionStatusRunning,
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:      "get completed session",
			sessionID: "session-456",
			serverResponse: &Session{
				SessionID: "session-456",
				Status:    SessionStatusFinished,
				PullRequest: &PullRequest{
					URL: "https://github.com/org/repo/pull/123",
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "session not found",
			sessionID:    "nonexistent",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				expectedPath := "/sessions/" + tt.sessionID
				if r.URL.Path != expectedPath {
					t.Errorf("expected %s, got %s", expectedPath, r.URL.Path)
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverResponse != nil {
					if err := json.NewEncoder(w).Encode(tt.serverResponse); err != nil {
						t.Errorf("failed to encode response: %v", err)
					}
				}
			}))
			defer server.Close()

			client, _ := NewClient("test-key")
			client = client.WithBaseURL(server.URL)

			session, err := client.GetSession(tt.sessionID)
			if tt.wantErr {
				if err == nil {
					t.Error("GetSession() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("GetSession() unexpected error: %v", err)
				return
			}
			if session.SessionID != tt.serverResponse.SessionID {
				t.Errorf("GetSession() SessionID = %v, want %v", session.SessionID, tt.serverResponse.SessionID)
			}
			if session.Status != tt.serverResponse.Status {
				t.Errorf("GetSession() Status = %v, want %v", session.Status, tt.serverResponse.Status)
			}
		})
	}
}

func TestSendMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		sessionID    string
		message      string
		serverStatus int
		wantErr      bool
	}{
		{
			name:         "send message successfully",
			sessionID:    "session-123",
			message:      "Please also add tests",
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "session not found",
			sessionID:    "nonexistent",
			message:      "Test message",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST, got %s", r.Method)
				}
				expectedPath := "/sessions/" + tt.sessionID + "/message"
				if r.URL.Path != expectedPath {
					t.Errorf("expected %s, got %s", expectedPath, r.URL.Path)
				}

				var req SendMessageRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("failed to decode request: %v", err)
				}
				if req.Message != tt.message {
					t.Errorf("expected message %q, got %q", tt.message, req.Message)
				}

				w.WriteHeader(tt.serverStatus)
			}))
			defer server.Close()

			client, _ := NewClient("test-key")
			client = client.WithBaseURL(server.URL)

			err := client.SendMessage(tt.sessionID, tt.message)
			if tt.wantErr {
				if err == nil {
					t.Error("SendMessage() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("SendMessage() unexpected error: %v", err)
			}
		})
	}
}

func TestTerminateSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		sessionID    string
		serverStatus int
		wantErr      bool
	}{
		{
			name:         "terminate successfully",
			sessionID:    "session-123",
			serverStatus: http.StatusOK,
			wantErr:      false,
		},
		{
			name:         "session not found",
			sessionID:    "nonexistent",
			serverStatus: http.StatusNotFound,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodDelete {
					t.Errorf("expected DELETE, got %s", r.Method)
				}
				expectedPath := "/sessions/" + tt.sessionID
				if r.URL.Path != expectedPath {
					t.Errorf("expected %s, got %s", expectedPath, r.URL.Path)
				}

				w.WriteHeader(tt.serverStatus)
			}))
			defer server.Close()

			client, _ := NewClient("test-key")
			client = client.WithBaseURL(server.URL)

			err := client.TerminateSession(tt.sessionID)
			if tt.wantErr {
				if err == nil {
					t.Error("TerminateSession() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("TerminateSession() unexpected error: %v", err)
			}
		})
	}
}

func TestListSessions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		serverResponse *ListSessionsResponse
		serverStatus   int
		wantErr        bool
		wantCount      int
	}{
		{
			name: "list sessions successfully",
			serverResponse: &ListSessionsResponse{
				Sessions: []Session{
					{SessionID: "session-1", Status: SessionStatusRunning},
					{SessionID: "session-2", Status: SessionStatusFinished},
				},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    2,
		},
		{
			name: "empty list",
			serverResponse: &ListSessionsResponse{
				Sessions: []Session{},
			},
			serverStatus: http.StatusOK,
			wantErr:      false,
			wantCount:    0,
		},
		{
			name:         "server error",
			serverStatus: http.StatusInternalServerError,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("expected GET, got %s", r.Method)
				}
				if r.URL.Path != "/sessions" {
					t.Errorf("expected /sessions, got %s", r.URL.Path)
				}

				w.WriteHeader(tt.serverStatus)
				if tt.serverResponse != nil {
					if err := json.NewEncoder(w).Encode(tt.serverResponse); err != nil {
						t.Errorf("failed to encode response: %v", err)
					}
				}
			}))
			defer server.Close()

			client, _ := NewClient("test-key")
			client = client.WithBaseURL(server.URL)

			sessions, err := client.ListSessions()
			if tt.wantErr {
				if err == nil {
					t.Error("ListSessions() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ListSessions() unexpected error: %v", err)
				return
			}
			if len(sessions) != tt.wantCount {
				t.Errorf("ListSessions() returned %d sessions, want %d", len(sessions), tt.wantCount)
			}
		})
	}
}

func TestSessionStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		status     SessionStatus
		isTerminal bool
	}{
		{SessionStatusRunning, false},
		{SessionStatusBlocked, false},
		{SessionStatusFinished, true},
		{SessionStatusStopped, true},
		{SessionStatusFailed, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.IsTerminal(); got != tt.isTerminal {
				t.Errorf("SessionStatus(%s).IsTerminal() = %v, want %v", tt.status, got, tt.isTerminal)
			}
		})
	}
}

func TestWaitForCompletion(t *testing.T) {
	t.Parallel()

	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		session := Session{
			SessionID: "session-123",
			Status:    SessionStatusRunning,
		}
		if callCount >= 3 {
			session.Status = SessionStatusFinished
			session.PullRequest = &PullRequest{URL: "https://github.com/org/repo/pull/1"}
		}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(session); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, _ := NewClient("test-key")
	client = client.WithBaseURL(server.URL)

	session, err := client.WaitForCompletion("session-123", 100*time.Millisecond, 10*time.Second)
	if err != nil {
		t.Errorf("WaitForCompletion() unexpected error: %v", err)
		return
	}
	if session.Status != SessionStatusFinished {
		t.Errorf("WaitForCompletion() Status = %v, want %v", session.Status, SessionStatusFinished)
	}
	if callCount < 3 {
		t.Errorf("WaitForCompletion() called GetSession %d times, expected at least 3", callCount)
	}
}

func TestWaitForCompletionTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := Session{
			SessionID: "session-123",
			Status:    SessionStatusRunning,
		}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(session); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client, _ := NewClient("test-key")
	client = client.WithBaseURL(server.URL)

	_, err := client.WaitForCompletion("session-123", 50*time.Millisecond, 200*time.Millisecond)
	if err == nil {
		t.Error("WaitForCompletion() expected timeout error, got nil")
	}
}
