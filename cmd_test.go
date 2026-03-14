package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- cmdLogout tests ---

func TestCmdLogout_RemovesConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Create a config file first
	cfg := &Config{Token: "test-token", APIURL: "https://example.com"}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify it exists
	path := filepath.Join(tmp, ".qamax", "config.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("config file should exist before logout")
	}

	// Run logout
	cmdLogout([]string{})

	// Verify config file was removed
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("config file should be removed after logout")
	}
}

func TestCmdLogout_AlreadyLoggedOut(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// No config file exists — should not panic
	cmdLogout([]string{})
}

// --- cmdProjects tests ---

func TestCmdProjects_Success(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/projects" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Error("missing Bearer token")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"projects": []map[string]interface{}{
				{"id": json.Number("1"), "name": "Project Alpha"},
				{"id": json.Number("2"), "name": "Project Beta"},
			},
		})
	}))
	defer server.Close()

	// Save config with token pointing to our test server
	cfg := &Config{Token: "test-token", APIURL: server.URL}
	_ = cfg.Save()

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmdProjects([]string{})

	w.Close()
	os.Stdout = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "Project Alpha") {
		t.Errorf("expected 'Project Alpha' in output, got: %s", output)
	}
	if !strings.Contains(output, "Project Beta") {
		t.Errorf("expected 'Project Beta' in output, got: %s", output)
	}
}

func TestCmdProjects_EmptyList(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"projects":[]}`)
	}))
	defer server.Close()

	cfg := &Config{Token: "test-token", APIURL: server.URL}
	_ = cfg.Save()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmdProjects([]string{})

	w.Close()
	os.Stdout = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "No projects found") {
		t.Errorf("expected 'No projects found' in output, got: %s", output)
	}
}

// --- cmdStatus tests ---

func TestCmdStatus_LoggedIn(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &Config{
		Token:   "my-token",
		APIURL:  "https://app.qualitymax.io",
		AgentID: "agent-123",
		APIKey:  "abcdef1234567890",
	}
	_ = cfg.Save()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmdStatus([]string{})

	w.Close()
	os.Stdout = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "logged in") {
		t.Errorf("expected 'logged in' in output, got: %s", output)
	}
	if !strings.Contains(output, "app.qualitymax.io") {
		t.Errorf("expected API URL in output, got: %s", output)
	}
	if !strings.Contains(output, "agent-123") {
		t.Errorf("expected agent ID in output, got: %s", output)
	}
	// API key should be masked
	if !strings.Contains(output, "abcd") {
		t.Errorf("expected masked API key in output, got: %s", output)
	}
	if !strings.Contains(output, "7890") {
		t.Errorf("expected last 4 chars of API key in output, got: %s", output)
	}
}

func TestCmdStatus_NotLoggedIn(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// No config file — default empty config

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmdStatus([]string{})

	w.Close()
	os.Stdout = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "not logged in") {
		t.Errorf("expected 'not logged in' in output, got: %s", output)
	}
	if !strings.Contains(output, "not configured") {
		t.Errorf("expected 'not configured' in output, got: %s", output)
	}
	if !strings.Contains(output, "not registered") {
		t.Errorf("expected 'not registered' in output, got: %s", output)
	}
}

func TestCmdStatus_ShortAPIKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &Config{
		Token:  "token",
		APIKey: "short", // less than 8 chars
	}
	_ = cfg.Save()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmdStatus([]string{})

	w.Close()
	os.Stdout = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "****") {
		t.Errorf("expected masked key '****' for short API key, got: %s", output)
	}
}

// --- cmdToken tests ---

func TestCmdToken_PrintsToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &Config{Token: "my-secret-token-value"}
	_ = cfg.Save()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmdToken([]string{})

	w.Close()
	os.Stdout = old

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if output != "my-secret-token-value" {
		t.Errorf("expected token value, got: %q", output)
	}
}

// --- printUsage tests ---

func TestPrintUsage(t *testing.T) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printUsage()

	w.Close()
	os.Stdout = old

	buf := make([]byte, 8192)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	if !strings.Contains(output, "qmax") {
		t.Errorf("expected 'qmax' in usage, got: %s", output)
	}
	if !strings.Contains(output, "run") {
		t.Errorf("expected 'run' command in usage")
	}
	if !strings.Contains(output, "login") {
		t.Errorf("expected 'login' command in usage")
	}
	if !strings.Contains(output, Version) {
		t.Errorf("expected version %s in usage", Version)
	}
}

// --- buildPlaywrightStorageState tests ---

func TestBuildPlaywrightStorageState_Empty(t *testing.T) {
	state := buildPlaywrightStorageState(nil, "https://example.com", nil)

	if len(state.Cookies) != 0 {
		t.Errorf("expected 0 cookies, got %d", len(state.Cookies))
	}
	if len(state.Origins) != 0 {
		t.Errorf("expected 0 origins, got %d", len(state.Origins))
	}
}

func TestBuildPlaywrightStorageState_WithLocalStorage(t *testing.T) {
	localStorage := []map[string]string{
		{"name": "session_id", "value": "abc123"},
	}
	state := buildPlaywrightStorageState(nil, "https://example.com/page/path", localStorage)

	if len(state.Origins) != 1 {
		t.Fatalf("expected 1 origin, got %d", len(state.Origins))
	}
	origin := state.Origins[0]["origin"]
	if origin != "https://example.com" {
		t.Errorf("origin: got %v, want 'https://example.com'", origin)
	}
}

// --- Config GetAPIBaseURL additional tests ---

func TestConfigGetAPIBaseURL_VariousFormats(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://app.qualitymax.io///", "https://app.qualitymax.io"},
		{"http://localhost:8000/app", "http://localhost:8000"},
		{"http://localhost:8000/app/", "http://localhost:8000"},
	}
	for _, tt := range tests {
		cfg := &Config{APIURL: tt.input}
		got := cfg.GetAPIBaseURL()
		if got != tt.want {
			t.Errorf("GetAPIBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
