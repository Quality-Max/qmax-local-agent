package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests run cmd functions that call os.Exit by executing a subprocess.
// The trick: we re-execute the test binary itself with a specific test function name,
// plus an env var to signal the subprocess to actually run the command.

// TestCmdLogoutSubprocess tests cmdLogout including the error path
func TestCmdLogoutSubprocess_RemovesConfig(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "logout" {
		// We're in the subprocess — run cmdLogout
		cmdLogout([]string{})
		return
	}

	tmp := t.TempDir()

	// Create config
	dir := filepath.Join(tmp, ".qamax")
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"token":"test"}`), 0600)

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdLogoutSubprocess_RemovesConfig$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=logout", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	output := string(out)
	if !strings.Contains(output, "Logged out") {
		t.Errorf("expected 'Logged out' in output, got: %s", output)
	}
}

func TestCmdLogoutSubprocess_NoConfig(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "logout_noconfig" {
		cmdLogout([]string{})
		return
	}

	tmp := t.TempDir()

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdLogoutSubprocess_NoConfig$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=logout_noconfig", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Already logged out") {
		t.Errorf("expected 'Already logged out' in output, got: %s", out)
	}
}

// TestCmdTokenSubprocess tests cmdToken with no config (should exit 1)
func TestCmdTokenSubprocess_NoLogin(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "token_nologin" {
		cmdToken([]string{})
		return
	}

	tmp := t.TempDir()

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdTokenSubprocess_NoLogin$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=token_nologin", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for no login")
	}
	if !strings.Contains(string(out), "not logged in") {
		t.Errorf("expected 'not logged in' in output, got: %s", out)
	}
}

func TestCmdTokenSubprocess_WithToken(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "token_ok" {
		cmdToken([]string{})
		return
	}

	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".qamax")
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"token":"my-secret-tok"}`), 0600)

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdTokenSubprocess_WithToken$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=token_ok", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "my-secret-tok") {
		t.Errorf("expected token in output, got: %s", out)
	}
}

// TestCmdProjectsSubprocess tests cmdProjects with no login
func TestCmdProjectsSubprocess_NoLogin(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "projects_nologin" {
		cmdProjects([]string{})
		return
	}

	tmp := t.TempDir()

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdProjectsSubprocess_NoLogin$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=projects_nologin", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for no login")
	}
	if !strings.Contains(string(out), "not logged in") {
		t.Errorf("expected 'not logged in' in output, got: %s", out)
	}
}

func TestCmdProjectsSubprocess_ServerError(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "projects_serverror" {
		cmdProjects([]string{})
		return
	}

	// Start a server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "server error")
	}))
	defer server.Close()

	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".qamax")
	_ = os.MkdirAll(dir, 0700)
	cfg := fmt.Sprintf(`{"token":"test-token","api_url":"%s"}`, server.URL)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0600)

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdProjectsSubprocess_ServerError$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=projects_serverror", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for server error")
	}
	_ = out
}

func TestCmdProjectsSubprocess_Success(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "projects_ok" {
		cmdProjects([]string{})
		return
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"projects": []map[string]interface{}{
				{"id": json.Number("1"), "name": "TestProject"},
			},
		})
	}))
	defer server.Close()

	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".qamax")
	_ = os.MkdirAll(dir, 0700)
	cfg := fmt.Sprintf(`{"token":"test-token","api_url":"%s"}`, server.URL)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte(cfg), 0600)

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdProjectsSubprocess_Success$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=projects_ok", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "TestProject") {
		t.Errorf("expected 'TestProject' in output, got: %s", out)
	}
}

// TestCmdStatusSubprocess tests the full cmdStatus flow
func TestCmdStatusSubprocess_NoConfig(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "status_noconfig" {
		cmdStatus([]string{})
		return
	}

	tmp := t.TempDir()

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdStatusSubprocess_NoConfig$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=status_noconfig", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	output := string(out)
	if !strings.Contains(output, "not logged in") {
		t.Errorf("expected 'not logged in' in output, got: %s", output)
	}
}

// TestCmdRunSubprocess tests cmdRun missing cloud-url
func TestCmdRunSubprocess_MissingURL(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "run_nourl" {
		cmdRun([]string{})
		return
	}

	tmp := t.TempDir()

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdRunSubprocess_MissingURL$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=run_nourl", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when cloud-url is missing")
	}
	output := string(out)
	if !strings.Contains(output, "cloud-url") {
		t.Errorf("expected 'cloud-url' in error output, got: %s", output)
	}
}

// TestMainSubprocess tests the main entry point
func TestMainSubprocess_Version(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_version" {
		os.Args = []string{"qamax-agent", "version"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_Version$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_version")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), Version) {
		t.Errorf("expected version %s in output, got: %s", Version, out)
	}
}

func TestMainSubprocess_Help(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_help" {
		os.Args = []string{"qamax-agent", "help"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_Help$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_help")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	output := string(out)
	if !strings.Contains(output, "qamax-agent") {
		t.Errorf("expected 'qamax-agent' in output, got: %s", output)
	}
}

func TestMainSubprocess_NoArgs(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_noargs" {
		os.Args = []string{"qamax-agent"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_NoArgs$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_noargs")

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when no args provided")
	}
	_ = out
}

func TestMainSubprocess_UnknownCommand(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_unknown" {
		os.Args = []string{"qamax-agent", "foobar"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_UnknownCommand$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_unknown")

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit for unknown command")
	}
	if !strings.Contains(string(out), "Unknown command") {
		t.Errorf("expected 'Unknown command' in output, got: %s", out)
	}
}

func TestMainSubprocess_BackwardCompatFlag(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_backcompat" {
		os.Args = []string{"qamax-agent", "--cloud-url", "http://localhost:9999"}
		main()
		return
	}

	tmp := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_BackwardCompatFlag$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_backcompat", "HOME="+tmp)

	// This will try to register and fail, but we test that the backward compat path works
	out, err := cmd.CombinedOutput()
	_ = err // Expected to fail (can't connect)
	output := string(out)
	// Should have attempted to run (backward compat triggers cmdRun)
	if strings.Contains(output, "Unknown command") {
		t.Error("backward compat flag should not produce 'Unknown command'")
	}
}

func TestMainSubprocess_Logout(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_logout" {
		os.Args = []string{"qamax-agent", "logout"}
		main()
		return
	}

	tmp := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_Logout$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_logout", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Already logged out") {
		t.Errorf("expected 'Already logged out', got: %s", out)
	}
}

func TestMainSubprocess_Status(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_status" {
		os.Args = []string{"qamax-agent", "status"}
		main()
		return
	}

	tmp := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_Status$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_status", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "not logged in") {
		t.Errorf("expected 'not logged in', got: %s", out)
	}
}

func TestMainSubprocess_Token(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_token" {
		os.Args = []string{"qamax-agent", "token"}
		main()
		return
	}

	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".qamax")
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"token":"main-test-tok"}`), 0600)

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_Token$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_token", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "main-test-tok") {
		t.Errorf("expected token in output, got: %s", out)
	}
}

func TestMainSubprocess_Projects(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_projects" {
		os.Args = []string{"qamax-agent", "projects"}
		main()
		return
	}

	tmp := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_Projects$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_projects", "HOME="+tmp)

	// Will exit 1 because no login
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when not logged in")
	}
	_ = out
}

func TestMainSubprocess_RunNoURL(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_run_nourl" {
		os.Args = []string{"qamax-agent", "run"}
		main()
		return
	}

	tmp := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_RunNoURL$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_run_nourl", "HOME="+tmp)

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when cloud-url is missing")
	}
	_ = out
}

func TestMainSubprocess_VersionFlag(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_vflag" {
		os.Args = []string{"qamax-agent", "--version"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_VersionFlag$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_vflag")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), Version) {
		t.Errorf("expected version in output, got: %s", out)
	}
}

func TestMainSubprocess_HelpFlag(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_hflag" {
		os.Args = []string{"qamax-agent", "-h"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_HelpFlag$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_hflag")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "qamax-agent") {
		t.Errorf("expected usage in output, got: %s", out)
	}
}

func TestMainSubprocess_VShortFlag(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_vshort" {
		os.Args = []string{"qamax-agent", "-v"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_VShortFlag$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_vshort")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), Version) {
		t.Errorf("expected version in output, got: %s", out)
	}
}

// --- cmdCapture subprocess tests ---

func TestCmdCaptureSubprocess_NoURL(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "capture_nourl" {
		cmdCapture([]string{"--project-id", "p1", "--name", "n1"})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdCaptureSubprocess_NoURL$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=capture_nourl", "HOME="+t.TempDir())

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when URL is missing")
	}
	if !strings.Contains(string(out), "URL argument is required") {
		t.Errorf("expected 'URL argument is required', got: %s", out)
	}
}

func TestCmdCaptureSubprocess_NoProjectID(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "capture_nopid" {
		cmdCapture([]string{"--name", "n1", "https://example.com"})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdCaptureSubprocess_NoProjectID$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=capture_nopid", "HOME="+t.TempDir())

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when project-id is missing")
	}
	if !strings.Contains(string(out), "project-id is required") {
		t.Errorf("expected '--project-id is required', got: %s", out)
	}
}

func TestCmdCaptureSubprocess_NoName(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "capture_noname" {
		cmdCapture([]string{"--project-id", "p1", "https://example.com"})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdCaptureSubprocess_NoName$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=capture_noname", "HOME="+t.TempDir())

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when name is missing")
	}
	if !strings.Contains(string(out), "name is required") {
		t.Errorf("expected '--name is required', got: %s", out)
	}
}

func TestCmdCaptureSubprocess_NotLoggedIn(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "capture_nologin" {
		cmdCapture([]string{"--project-id", "p1", "--name", "n1", "https://example.com"})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdCaptureSubprocess_NotLoggedIn$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=capture_nologin", "HOME="+t.TempDir())

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when not logged in")
	}
	if !strings.Contains(string(out), "not logged in") {
		t.Errorf("expected 'not logged in', got: %s", out)
	}
}

func TestCmdCaptureSubprocess_URLViaFlag(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "capture_urlflag" {
		cmdCapture([]string{"--project-id", "p1", "--name", "n1", "--url", "https://example.com"})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdCaptureSubprocess_URLViaFlag$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=capture_urlflag", "HOME="+t.TempDir())

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit when not logged in")
	}
	// Should get past URL validation and fail on login check
	if !strings.Contains(string(out), "not logged in") {
		t.Errorf("expected 'not logged in', got: %s", out)
	}
}

func TestMainSubprocess_Capture(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "main_capture" {
		os.Args = []string{"qamax-agent", "capture"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSubprocess_Capture$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=main_capture", "HOME="+t.TempDir())

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit")
	}
	if !strings.Contains(string(out), "URL argument is required") {
		t.Errorf("expected URL error from capture, got: %s", out)
	}
}

// --- cmdLogin tests (non-subprocess, test the token flow directly) ---

func TestCmdLogin_TokenCallback(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Run cmdLogin in a goroutine with our port
	done := make(chan struct{})
	go func() {
		defer close(done)
		cmdLogin([]string{"--port", fmt.Sprintf("%d", port), "--api-url", "https://example.com"})
	}()

	// Wait for the server to start
	var connected bool
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			conn.Close()
			connected = true
			break
		}
	}
	if !connected {
		t.Fatal("login server did not start in time")
	}

	// Simulate browser callback
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback?token=test-jwt-token-1234567890", port)
	resp, err := http.Get(callbackURL)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("callback status: got %d, want 200", resp.StatusCode)
	}

	// Wait for cmdLogin to finish
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cmdLogin did not finish after callback")
	}

	// Verify config was saved
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.Token != "test-jwt-token-1234567890" {
		t.Errorf("token: got %q, want 'test-jwt-token-1234567890'", cfg.Token)
	}
	if cfg.APIURL != "https://example.com" {
		t.Errorf("APIURL: got %q, want 'https://example.com'", cfg.APIURL)
	}
}

func TestCmdLogin_RejectsShortToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	go func() {
		cmdLogin([]string{"--port", fmt.Sprintf("%d", port), "--api-url", "https://example.com"})
	}()

	// Wait for server
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			conn.Close()
			break
		}
	}

	// Send short token — should be rejected
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback?token=short", port))
	if err != nil {
		t.Fatalf("callback failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for short token, got %d", resp.StatusCode)
	}
}

func TestCmdLogin_RejectsMissingToken(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	go func() {
		cmdLogin([]string{"--port", fmt.Sprintf("%d", port), "--api-url", "https://example.com"})
	}()

	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			conn.Close()
			break
		}
	}

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/callback", port))
	if err != nil {
		t.Fatalf("callback failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for missing token, got %d", resp.StatusCode)
	}
}

func TestCmdLogin_RejectsPostMethod(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("cannot find free port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	go func() {
		cmdLogin([]string{"--port", fmt.Sprintf("%d", port), "--api-url", "https://example.com"})
	}()

	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			conn.Close()
			break
		}
	}

	resp, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/callback?token=validtoken1234567890", port), "", nil)
	if err != nil {
		t.Fatalf("callback failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", resp.StatusCode)
	}
}

func TestCmdRunSubprocess_WithConfig(t *testing.T) {
	if os.Getenv("RUN_CMD_TEST") == "run_withconfig" {
		cmdRun([]string{"--cloud-url", "http://127.0.0.1:1"})
		return
	}

	tmp := t.TempDir()
	dir := filepath.Join(tmp, ".qamax")
	_ = os.MkdirAll(dir, 0700)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"api_url":"http://127.0.0.1:1","agent_id":"test-id","api_key":"test-key"}`), 0600)

	cmd := exec.Command(os.Args[0], "-test.run=^TestCmdRunSubprocess_WithConfig$")
	cmd.Env = append(os.Environ(), "RUN_CMD_TEST=run_withconfig", "HOME="+tmp)

	// Will fail because it can't connect, but exercises the code path
	out, err := cmd.CombinedOutput()
	_ = err // Expected to fail (can't register)
	_ = out
}
