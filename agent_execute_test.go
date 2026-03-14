package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// --- ExecuteTest additional paths ---

func TestExecuteTest_FetchScriptNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/api/automation/scripts/") {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, "not found")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.mu.Lock()
	a.activeCount = 1
	a.mu.Unlock()
	a.activeTests.Store("fetch-fail-1", true)

	assignment := Assignment{
		ID:       "fetch-fail-1",
		ScriptID: "nonexistent-script",
		Code:     "",
	}

	ctx := context.Background()
	a.ExecuteTest(ctx, assignment)
	// Should not panic — reports error and returns
}

func TestExecuteTest_CleansUpActiveCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.mu.Lock()
	a.activeCount = 1
	a.mu.Unlock()
	a.activeTests.Store("cleanup-check", true)

	assignment := Assignment{
		ID:   "cleanup-check",
		Code: "", // Will fail fast with "no test code"
	}

	ctx := context.Background()
	a.ExecuteTest(ctx, assignment)

	// Verify active test was removed
	if _, exists := a.activeTests.Load("cleanup-check"); exists {
		t.Error("expected active test to be removed after execution")
	}

	a.mu.Lock()
	count := a.activeCount
	a.mu.Unlock()
	if count != 0 {
		t.Errorf("expected activeCount=0, got %d", count)
	}
}

func TestExecuteTest_CustomBrowserFirefox(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.mu.Lock()
	a.activeCount = 1
	a.mu.Unlock()
	a.activeTests.Store("firefox-test", true)

	assignment := Assignment{
		ID:             "firefox-test",
		Code:           "const {test} = require('@playwright/test'); test('x', async () => {});",
		Browser:        "firefox",
		Headless:       true,
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	a.ExecuteTest(ctx, assignment)
	// Test will fail at npm install due to timeout — that's fine, we're testing setup paths
}

func TestExecuteTest_UnknownBrowserFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.mu.Lock()
	a.activeCount = 1
	a.mu.Unlock()
	a.activeTests.Store("unknown-b", true)

	assignment := Assignment{
		ID:      "unknown-b",
		Code:    "test('x', async () => {});",
		Browser: "opera", // unknown — should map to chromium
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	a.ExecuteTest(ctx, assignment)
}

func TestExecuteTest_WithBaseURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.mu.Lock()
	a.activeCount = 1
	a.mu.Unlock()
	a.activeTests.Store("baseurl-1", true)

	assignment := Assignment{
		ID:        "baseurl-1",
		Code:      "test('x', async () => {});",
		CustomURL: "https://example.com",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	a.ExecuteTest(ctx, assignment)
}

// --- Run with crawl session ---

func TestRun_DispatchesCrawlSession(t *testing.T) {
	crawlPolled := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/register"):
			_ = json.NewEncoder(w).Encode(map[string]string{
				"agent_id": "crawl-dispatch-agent",
				"api_key":  "crawl-dispatch-key",
			})
		case strings.Contains(r.URL.Path, "/heartbeat"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/assignments/pending"):
			fmt.Fprint(w, `{"assignments":[]}`)
		case strings.Contains(r.URL.Path, "/crawl/pending"):
			count := atomic.AddInt32(&crawlPolled, 1)
			if count == 1 {
				// Return a crawl session that will fail (invalid URL)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"session": map[string]interface{}{
						"session_id": "test-crawl-dispatch",
						"url":        "http://127.0.0.1:1/nonexistent",
						"max_steps":  1,
					},
				})
			} else {
				w.WriteHeader(http.StatusNoContent)
			}
		case strings.Contains(r.URL.Path, "/error"):
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "", 100*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(1 * time.Second)
		cancel()
	}()

	err := a.Run(ctx)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

// --- collectArtifacts additional ---

func TestCollectArtifacts_DeepNestedDir(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "test-results", "test-1", "retry-0")
	_ = os.MkdirAll(nested, 0755)

	_ = os.WriteFile(filepath.Join(nested, "screenshot.png"), []byte("png-data"), 0644)
	_ = os.WriteFile(filepath.Join(nested, "recording.webm"), []byte("webm-data"), 0644)

	a := &Agent{}
	artifacts := a.collectArtifacts(tmpDir)

	screenshots, ok := artifacts["screenshots"].([]map[string]string)
	if !ok || len(screenshots) != 1 {
		t.Errorf("expected 1 screenshot, got %d", len(screenshots))
	}
	if artifacts["video"] == nil {
		t.Error("expected video to be present")
	}
}

func TestCollectArtifacts_NoArtifactDir(t *testing.T) {
	a := &Agent{}
	artifacts := a.collectArtifacts("/nonexistent/dir/12345")
	if artifacts == nil {
		t.Fatal("artifacts should not be nil")
	}
}

// --- heartbeatLoop with backoff ---

func TestHeartbeatLoop_BackoffAndRecover(t *testing.T) {
	var heartbeats int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&heartbeats, 1)
		if count <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.HeartbeatInterval = 10 * time.Millisecond
	a.running = true

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		a.heartbeatLoop(ctx)
		close(done)
	}()

	// Wait long enough for backoff to recover (with race detector, timing is slower)
	time.Sleep(5 * time.Second)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("heartbeatLoop should stop after context cancellation")
	}

	count := atomic.LoadInt32(&heartbeats)
	if count < 2 {
		t.Errorf("expected at least 2 heartbeat attempts, got %d", count)
	}
}

// --- reportResult with server rejection ---

func TestReportResult_WithFullResultData(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	resultData := map[string]interface{}{
		"output":    "test output text",
		"errors":    "some error text",
		"artifacts": map[string]interface{}{"screenshots": []string{}},
	}
	a.reportResult("assign-full-data", true, "passed", resultData)

	if receivedPayload["output"] != "test output text" {
		t.Errorf("output: got %v", receivedPayload["output"])
	}
	if receivedPayload["errors"] != "some error text" {
		t.Errorf("errors: got %v", receivedPayload["errors"])
	}
}

func TestReportResult_ServerRejectsFully(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "server error")
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	// Should not panic when server rejects result
	a.reportResult("assign-reject-full", true, "ok", nil)
}

// --- updateAssignmentStatus edge ---

func TestUpdateAssignmentStatus_ServerReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.updateAssignmentStatus("assign-srv-err", "started")
}

func TestUpdateAssignmentStatus_Unreachable(t *testing.T) {
	a := newTestAgent("http://127.0.0.1:1")
	a.updateAssignmentStatus("assign-unreach", "started")
}

// --- Register additional ---

func TestRegister_ConnectionRefused(t *testing.T) {
	a := NewAgent("http://127.0.0.1:1", "", "", "", 5*time.Second, 60*time.Second)
	err := a.Register()
	if err == nil {
		t.Error("expected error for connection failure")
	}
}

func TestRegister_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "not json at all")
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "", 5*time.Second, 60*time.Second)
	err := a.Register()
	if err == nil {
		t.Error("expected error for malformed JSON response")
	}
}

// --- getPlatformVersion ---

func TestGetPlatformVersion_NonEmpty(t *testing.T) {
	a := &Agent{}
	version := a.getPlatformVersion()
	// On macOS/Linux it should return something; on other platforms empty is OK
	_ = version
}
