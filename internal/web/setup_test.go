package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testSetupToken = "test-setup-token"

func newSetupTestServer() http.Handler {
	return newSetupMuxWithToken(testSetupToken)
}

func setupRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321"
	req.Host = "localhost:8080"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(setupTokenHeader, testSetupToken)
	return req
}

func TestSetupAPI_RequiresToken(t *testing.T) {
	handler := newSetupTestServer()
	for _, token := range []string{"", "wrong-token"} {
		req := setupRequest(http.MethodPost, "/api/rig/add", `{"name":"x","gitUrl":"https://github.com/o/r.git"}`)
		if token == "" {
			req.Header.Del(setupTokenHeader)
		} else {
			req.Header.Set(setupTokenHeader, token)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("token %q: status = %d, want %d", token, rec.Code, http.StatusForbidden)
		}
	}
}

func TestSetupAPI_RejectsAllEndpointsWithoutToken(t *testing.T) {
	handler := newSetupTestServer()
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/install"},
		{http.MethodPost, "/api/rig/add"},
		{http.MethodPost, "/api/launch"},
		{http.MethodPost, "/api/check-workspace"},
		{http.MethodGet, "/api/status"},
	} {
		req := setupRequest(tc.method, tc.path, `{}`)
		req.Header.Del(setupTokenHeader)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want %d", tc.method, tc.path, rec.Code, http.StatusForbidden)
		}
	}
}

func TestSetupMux_RejectsNonLoopbackClients(t *testing.T) {
	handler := newSetupTestServer()
	for _, path := range []string{"/", "/api/status"} {
		req := setupRequest(http.MethodGet, path, "")
		req.RemoteAddr = "203.0.113.7:40000"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s from remote addr: status = %d, want %d", path, rec.Code, http.StatusForbidden)
		}
	}
}

func TestSetupMux_RejectsNonLocalHostHeader(t *testing.T) {
	handler := newSetupTestServer()
	for _, path := range []string{"/", "/api/status"} {
		req := setupRequest(http.MethodGet, path, "")
		req.Host = "attacker.example:8080"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s with foreign Host: status = %d, want %d", path, rec.Code, http.StatusForbidden)
		}
	}
}

func TestSetupMux_AllowsLocalHostVariants(t *testing.T) {
	handler := newSetupTestServer()
	for _, tc := range []struct{ remote, host string }{
		{"127.0.0.1:1", "localhost:8080"},
		{"127.0.0.1:1", "127.0.0.1:8080"},
		{"[::1]:1", "[::1]:8080"},
		{"[::1]:1", "localhost"},
	} {
		req := setupRequest(http.MethodGet, "/", "")
		req.RemoteAddr = tc.remote
		req.Host = tc.host
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("remote %s host %s: status = %d, want %d", tc.remote, tc.host, rec.Code, http.StatusOK)
		}
	}
}

func TestSetupAPI_NoWildcardCORS(t *testing.T) {
	handler := newSetupTestServer()
	req := setupRequest(http.MethodOptions, "/api/rig/add", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestSetupPage_EmbedsToken(t *testing.T) {
	handler := newSetupTestServer()
	req := setupRequest(http.MethodGet, "/", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `content="`+testSetupToken+`"`) {
		t.Error("setup page does not embed the setup token")
	}
	if strings.Contains(body, setupTokenPlaceholder) {
		t.Error("setup page still contains the token placeholder")
	}
}

func TestSetupAPI_RigAddRejectsUnsafeInput(t *testing.T) {
	handler := newSetupTestServer()
	tests := []struct {
		name, gitURL string
	}{
		{"x", `ext::sh -c \"id > /tmp/pwned\"`},
		{"x", "fd::3"},
		{"x", "--upload-pack=id"},
		{"x", "file:///etc/repo"},
		{"x", "/home/user/private-repo"},
		{"x", "http://example.com/repo.git"},
		{"--adopt", "https://github.com/o/r.git"},
		{"bad name", "https://github.com/o/r.git"},
		{"../escape", "https://github.com/o/r.git"},
	}
	for _, tt := range tests {
		body, _ := json.Marshal(RigAddRequest{Name: tt.name, GitURL: tt.gitURL})
		req := setupRequest(http.MethodPost, "/api/rig/add", string(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("name %q url %q: status = %d, want %d", tt.name, tt.gitURL, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestValidateSetupGitURL(t *testing.T) {
	for _, u := range []string{
		"https://github.com/owner/repo.git",
		"ssh://git@github.com/owner/repo.git",
		"git@github.com:owner/repo.git",
	} {
		if err := validateSetupGitURL(u); err != nil {
			t.Errorf("validateSetupGitURL(%q) = %v, want nil", u, err)
		}
	}
	for _, u := range []string{
		"", "ext::sh -c id", "EXT::sh", "-u", "file:///x", "/abs/path", "./rel", "git://host/repo",
		"http://host/repo", "git@host:-oProxyCommand=id", "-oProxyCommand=id@host:repo",
		"https://host/repo\nx",
	} {
		if err := validateSetupGitURL(u); err == nil {
			t.Errorf("validateSetupGitURL(%q) = nil, want error", u)
		}
	}
}
