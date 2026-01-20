package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/devin"
)

func TestDevinAgentCommand_RequiresAPIKey(t *testing.T) {
	// Unset the API key to test validation
	originalKey := os.Getenv("DEVIN_API_KEY")
	os.Unsetenv("DEVIN_API_KEY")
	defer func() {
		if originalKey != "" {
			os.Setenv("DEVIN_API_KEY", originalKey)
		}
	}()

	// Create a mock server that should never be called
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("API should not be called when API key is missing")
	}))
	defer server.Close()

	// Test that the command fails without an API key
	err := runDevinAgentWithOptions(devinAgentOptions{
		prompt:  "test prompt",
		baseURL: server.URL,
	})
	if err == nil {
		t.Error("expected error when DEVIN_API_KEY is not set")
	}
}

func TestDevinAgentCommand_RequiresPrompt(t *testing.T) {
	// Set a dummy API key
	originalKey := os.Getenv("DEVIN_API_KEY")
	os.Setenv("DEVIN_API_KEY", "test-key")
	defer func() {
		if originalKey != "" {
			os.Setenv("DEVIN_API_KEY", originalKey)
		} else {
			os.Unsetenv("DEVIN_API_KEY")
		}
	}()

	// Test that the command fails without a prompt
	err := runDevinAgentWithOptions(devinAgentOptions{
		prompt:  "",
		baseURL: "http://localhost",
	})
	if err == nil {
		t.Error("expected error when prompt is empty")
	}
}

func TestDevinAgentCommand_CreatesSession(t *testing.T) {
	// Set up mock server
	sessionCreated := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/sessions":
			sessionCreated = true
			// Verify authorization header
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-api-key" {
				t.Errorf("expected Authorization header 'Bearer test-api-key', got %q", auth)
			}

			// Verify request body
			var req devin.CreateSessionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request body: %v", err)
			}
			if req.Prompt != "test prompt" {
				t.Errorf("expected prompt 'test prompt', got %q", req.Prompt)
			}

			// Return session response
			resp := devin.Session{
				SessionID: "test-session-123",
				Status:    devin.SessionStatusRunning,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case r.Method == "GET" && r.URL.Path == "/sessions/test-session-123":
			// Return finished session
			resp := devin.Session{
				SessionID: "test-session-123",
				Status:    devin.SessionStatusFinished,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Set API key
	originalKey := os.Getenv("DEVIN_API_KEY")
	os.Setenv("DEVIN_API_KEY", "test-api-key")
	defer func() {
		if originalKey != "" {
			os.Setenv("DEVIN_API_KEY", originalKey)
		} else {
			os.Unsetenv("DEVIN_API_KEY")
		}
	}()

	// Capture output
	var stdout bytes.Buffer

	// Run command
	err := runDevinAgentWithOptions(devinAgentOptions{
		prompt:       "test prompt",
		baseURL:      server.URL,
		pollInterval: 10 * time.Millisecond,
		timeout:      1 * time.Second,
		stdout:       &stdout,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !sessionCreated {
		t.Error("session was not created")
	}
}

func TestDevinAgentCommand_HandlesSessionTimeout(t *testing.T) {
	// Set up mock server that always returns running status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/sessions":
			resp := devin.Session{
				SessionID: "test-session-123",
				Status:    devin.SessionStatusRunning,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case r.Method == "GET" && r.URL.Path == "/sessions/test-session-123":
			// Always return running - never finishes
			resp := devin.Session{
				SessionID: "test-session-123",
				Status:    devin.SessionStatusRunning,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Set API key
	originalKey := os.Getenv("DEVIN_API_KEY")
	os.Setenv("DEVIN_API_KEY", "test-api-key")
	defer func() {
		if originalKey != "" {
			os.Setenv("DEVIN_API_KEY", originalKey)
		} else {
			os.Unsetenv("DEVIN_API_KEY")
		}
	}()

	var stdout bytes.Buffer

	// Run command with short timeout
	err := runDevinAgentWithOptions(devinAgentOptions{
		prompt:       "test prompt",
		baseURL:      server.URL,
		pollInterval: 10 * time.Millisecond,
		timeout:      50 * time.Millisecond,
		stdout:       &stdout,
	})

	// Should timeout
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestDevinAgentCommand_HandlesSessionFailure(t *testing.T) {
	// Set up mock server that returns failed status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/sessions":
			resp := devin.Session{
				SessionID: "test-session-123",
				Status:    devin.SessionStatusRunning,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case r.Method == "GET" && r.URL.Path == "/sessions/test-session-123":
			// Return failed status
			resp := devin.Session{
				SessionID: "test-session-123",
				Status:    devin.SessionStatusFailed,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Set API key
	originalKey := os.Getenv("DEVIN_API_KEY")
	os.Setenv("DEVIN_API_KEY", "test-api-key")
	defer func() {
		if originalKey != "" {
			os.Setenv("DEVIN_API_KEY", originalKey)
		} else {
			os.Unsetenv("DEVIN_API_KEY")
		}
	}()

	var stdout bytes.Buffer

	// Run command
	err := runDevinAgentWithOptions(devinAgentOptions{
		prompt:       "test prompt",
		baseURL:      server.URL,
		pollInterval: 10 * time.Millisecond,
		timeout:      1 * time.Second,
		stdout:       &stdout,
	})

	// Should report failure
	if err == nil {
		t.Error("expected error for failed session")
	}
}

func TestDevinAgentCommand_OutputsSessionID(t *testing.T) {
	// Set up mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/sessions":
			resp := devin.Session{
				SessionID: "unique-session-id-456",
				Status:    devin.SessionStatusRunning,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case r.Method == "GET" && r.URL.Path == "/sessions/unique-session-id-456":
			resp := devin.Session{
				SessionID: "unique-session-id-456",
				Status:    devin.SessionStatusFinished,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Set API key
	originalKey := os.Getenv("DEVIN_API_KEY")
	os.Setenv("DEVIN_API_KEY", "test-api-key")
	defer func() {
		if originalKey != "" {
			os.Setenv("DEVIN_API_KEY", originalKey)
		} else {
			os.Unsetenv("DEVIN_API_KEY")
		}
	}()

	var stdout bytes.Buffer

	// Run command
	err := runDevinAgentWithOptions(devinAgentOptions{
		prompt:       "test prompt",
		baseURL:      server.URL,
		pollInterval: 10 * time.Millisecond,
		timeout:      1 * time.Second,
		stdout:       &stdout,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Check that session ID was output
	output := stdout.String()
	if !bytes.Contains([]byte(output), []byte("unique-session-id-456")) {
		t.Errorf("expected output to contain session ID, got: %s", output)
	}
}

func TestDevinAgentCommand_WithPlaybook(t *testing.T) {
	// Set up mock server
	playbookUsed := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/sessions":
			var req devin.CreateSessionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request body: %v", err)
			}
			playbookUsed = req.PlaybookID

			resp := devin.Session{
				SessionID:  "test-session-123",
				Status:     devin.SessionStatusRunning,
				PlaybookID: req.PlaybookID,
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		case r.Method == "GET" && r.URL.Path == "/sessions/test-session-123":
			resp := devin.Session{
				SessionID: "test-session-123",
				Status:    devin.SessionStatusFinished,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Set API key
	originalKey := os.Getenv("DEVIN_API_KEY")
	os.Setenv("DEVIN_API_KEY", "test-api-key")
	defer func() {
		if originalKey != "" {
			os.Setenv("DEVIN_API_KEY", originalKey)
		} else {
			os.Unsetenv("DEVIN_API_KEY")
		}
	}()

	var stdout bytes.Buffer

	// Run command with playbook
	err := runDevinAgentWithOptions(devinAgentOptions{
		prompt:       "test prompt",
		playbookID:   "my-playbook-123",
		baseURL:      server.URL,
		pollInterval: 10 * time.Millisecond,
		timeout:      1 * time.Second,
		stdout:       &stdout,
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if playbookUsed != "my-playbook-123" {
		t.Errorf("expected playbook 'my-playbook-123', got %q", playbookUsed)
	}
}
