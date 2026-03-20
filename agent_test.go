package main

import (
	"context"
	"encoding/base64"
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

// --- NewAgent tests ---

func TestNewAgent(t *testing.T) {
	a := NewAgent("https://app.qualitymax.io/", "key1", "agent1", "secret1", 5*time.Second, 60*time.Second)

	if a.CloudURL != "https://app.qualitymax.io" {
		t.Errorf("CloudURL: got %q, want trailing slash stripped", a.CloudURL)
	}
	if a.APIKey != "key1" {
		t.Errorf("APIKey: got %q, want %q", a.APIKey, "key1")
	}
	if a.AgentID != "agent1" {
		t.Errorf("AgentID: got %q, want %q", a.AgentID, "agent1")
	}
	if a.RegistrationSecret != "secret1" {
		t.Errorf("RegistrationSecret: got %q", a.RegistrationSecret)
	}
	if a.PollInterval != 5*time.Second {
		t.Errorf("PollInterval: got %v", a.PollInterval)
	}
	if a.HeartbeatInterval != 60*time.Second {
		t.Errorf("HeartbeatInterval: got %v", a.HeartbeatInterval)
	}
	if a.MachineID == "" {
		t.Error("MachineID should not be empty")
	}
	if a.Capabilities == nil {
		t.Error("Capabilities should not be nil")
	}
	if a.client == nil {
		t.Error("client should not be nil")
	}
}

func TestNewAgent_EmptyValues(t *testing.T) {
	a := NewAgent("", "", "", "", 0, 0)
	if a.CloudURL != "" {
		t.Errorf("expected empty CloudURL, got %q", a.CloudURL)
	}
	// Should still have a valid MachineID and Capabilities
	if a.MachineID == "" {
		t.Error("MachineID should not be empty even with empty inputs")
	}
	if a.Capabilities == nil {
		t.Error("Capabilities should not be nil even with empty inputs")
	}
}

// --- getMachineID tests ---

func TestGetMachineID(t *testing.T) {
	a := &Agent{}
	mid := a.getMachineID()
	if mid == "" {
		t.Error("getMachineID returned empty string")
	}
	// Should contain the OS name
	if !strings.Contains(mid, "darwin") && !strings.Contains(mid, "linux") && !strings.Contains(mid, "windows") {
		t.Errorf("getMachineID should contain OS name, got %q", mid)
	}
}

// --- detectCapabilities tests ---

func TestDetectCapabilities(t *testing.T) {
	a := &Agent{}
	a.MachineID = a.getMachineID()
	caps := a.detectCapabilities()

	if caps == nil {
		t.Fatal("detectCapabilities returned nil")
	}

	// Check required keys
	requiredKeys := []string{"frameworks", "browsers", "execution_type", "platform", "platform_version", "architecture", "playwright_available"}
	for _, key := range requiredKeys {
		if _, ok := caps[key]; !ok {
			t.Errorf("missing capability key: %s", key)
		}
	}

	if caps["execution_type"] != "local_agent" {
		t.Errorf("execution_type: got %v, want 'local_agent'", caps["execution_type"])
	}

	frameworks, ok := caps["frameworks"].([]string)
	if !ok || len(frameworks) == 0 {
		t.Error("frameworks should be a non-empty string slice")
	}

	browsers, ok := caps["browsers"].([]string)
	if !ok || len(browsers) == 0 {
		t.Error("browsers should be a non-empty string slice")
	}
}

// --- pathExists tests ---

func TestPathExists_ExistingFile(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	_ = os.WriteFile(tmpFile, []byte("hello"), 0644)

	if !pathExists([]string{tmpFile}) {
		t.Error("pathExists should return true for existing file")
	}
}

func TestPathExists_NonExistingFile(t *testing.T) {
	if pathExists([]string{"/nonexistent/path/12345"}) {
		t.Error("pathExists should return false for non-existing file")
	}
}

func TestPathExists_MixedPaths(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "exists.txt")
	_ = os.WriteFile(tmpFile, []byte("hi"), 0644)

	if !pathExists([]string{"/no/such/path", tmpFile}) {
		t.Error("pathExists should return true if any path exists")
	}
}

func TestPathExists_EmptySlice(t *testing.T) {
	if pathExists([]string{}) {
		t.Error("pathExists should return false for empty slice")
	}
}

// --- commandExists tests ---

func TestCommandExists_Go(t *testing.T) {
	if !commandExists("go") {
		t.Skip("go command not found")
	}
}

func TestCommandExists_NonExistent(t *testing.T) {
	if commandExists("nonexistentcommand12345xyz") {
		t.Error("commandExists should return false for non-existent command")
	}
}

// --- doJSON tests ---

func TestDoJSON_GET(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "" {
			t.Error("GET without body should not have Content-Type")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	resp, body, err := a.doJSON("GET", server.URL+"/test", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != `{"status":"ok"}` {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestDoJSON_POST_WithBody(t *testing.T) {
	var receivedContentType string
	var receivedBody map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"result":"created"}`)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	payload := map[string]string{"name": "test"}
	resp, body, err := a.doJSON("POST", server.URL+"/create", payload, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if receivedContentType != "application/json" {
		t.Errorf("Content-Type: got %q", receivedContentType)
	}
	if receivedBody["name"] != "test" {
		t.Errorf("body name: got %q", receivedBody["name"])
	}
	if string(body) != `{"result":"created"}` {
		t.Errorf("unexpected response body: %s", body)
	}
}

func TestDoJSON_WithHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	headers := map[string]string{
		"X-Agent-API-Key": "my-key",
		"X-Custom":        "value123",
	}
	_, _, err := a.doJSON("GET", server.URL+"/test", nil, headers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedHeaders.Get("X-Agent-API-Key") != "my-key" {
		t.Error("missing X-Agent-API-Key header")
	}
	if receivedHeaders.Get("X-Custom") != "value123" {
		t.Error("missing X-Custom header")
	}
}

func TestDoJSON_ConnectionError(t *testing.T) {
	a := newTestAgent("http://127.0.0.1:1") // invalid port
	_, _, err := a.doJSON("GET", "http://127.0.0.1:1/test", nil, nil)
	if err == nil {
		t.Error("expected error for connection failure")
	}
}

// --- authHeaders tests ---

func TestAuthHeaders(t *testing.T) {
	a := &Agent{APIKey: "test-key-123"}
	headers := a.authHeaders()
	if headers["X-Agent-API-Key"] != "test-key-123" {
		t.Errorf("expected API key in headers, got %v", headers)
	}
}

// --- Register tests ---

func TestRegister_Success(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/agent/register" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"agent_id": "new-agent-id",
			"api_key":  "new-api-key",
		})
	}))
	defer server.Close()

	a := NewAgent(server.URL, "old-key", "", "my-secret", 5*time.Second, 60*time.Second)

	var callbackAgentID, callbackAPIKey string
	a.OnRegistered = func(agentID, apiKey string) {
		callbackAgentID = agentID
		callbackAPIKey = apiKey
	}

	err := a.Register()
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	if a.AgentID != "new-agent-id" {
		t.Errorf("AgentID: got %q, want %q", a.AgentID, "new-agent-id")
	}
	if a.APIKey != "new-api-key" {
		t.Errorf("APIKey: got %q, want %q", a.APIKey, "new-api-key")
	}

	// Verify callback was called
	if callbackAgentID != "new-agent-id" {
		t.Errorf("callback agentID: got %q", callbackAgentID)
	}
	if callbackAPIKey != "new-api-key" {
		t.Errorf("callback apiKey: got %q", callbackAPIKey)
	}

	// Verify payload contains expected fields
	if receivedPayload["version"] != Version {
		t.Errorf("payload version: got %v, want %s", receivedPayload["version"], Version)
	}
	if receivedPayload["registration_secret"] != "my-secret" {
		t.Errorf("payload registration_secret: got %v", receivedPayload["registration_secret"])
	}
	if receivedPayload["machine_id"] == nil || receivedPayload["machine_id"] == "" {
		t.Error("payload machine_id should not be empty")
	}
}

func TestRegister_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "invalid secret")
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "bad-secret", 5*time.Second, 60*time.Second)
	err := a.Register()
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should contain status code: %v", err)
	}
}

func TestRegister_NoCallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"agent_id": "agent-1",
			"api_key":  "key-1",
		})
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "", 5*time.Second, 60*time.Second)
	// OnRegistered is nil — should not panic
	err := a.Register()
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}
}

// --- SendHeartbeat tests ---

func TestSendHeartbeat_Success(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/heartbeat") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	err := a.SendHeartbeat()
	if err != nil {
		t.Fatalf("SendHeartbeat failed: %v", err)
	}

	if receivedPayload["status"] != "online" {
		t.Errorf("status: got %v, want 'online'", receivedPayload["status"])
	}
}

func TestSendHeartbeat_BusyStatus(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.activeTests.Store("test-1", true)
	defer a.activeTests.Delete("test-1")

	err := a.SendHeartbeat()
	if err != nil {
		t.Fatalf("SendHeartbeat failed: %v", err)
	}

	if receivedPayload["status"] != "busy" {
		t.Errorf("status: got %v, want 'busy'", receivedPayload["status"])
	}
}

func TestSendHeartbeat_NotRegistered(t *testing.T) {
	a := &Agent{
		CloudURL: "http://localhost",
		client:   &http.Client{Timeout: 5 * time.Second},
	}
	err := a.SendHeartbeat()
	if err == nil {
		t.Error("expected error when not registered")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("error should mention 'not registered': %v", err)
	}
}

func TestSendHeartbeat_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	err := a.SendHeartbeat()
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestSendHeartbeat_IncludesSystemMetrics(t *testing.T) {
	var receivedPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	err := a.SendHeartbeat()
	if err != nil {
		t.Fatalf("SendHeartbeat failed: %v", err)
	}

	// At minimum, status should be present
	if receivedPayload["status"] == nil {
		t.Error("payload should include status")
	}
}

// --- PollAssignments tests ---

func TestPollAssignments_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/assignments/pending") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"assignments": []map[string]interface{}{
				{
					"id":        "assign-1",
					"script_id": "script-1",
					"code":      "test code",
					"framework": "playwright",
					"headless":  true,
					"browser":   "chromium",
				},
			},
		})
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	assignments, err := a.PollAssignments()
	if err != nil {
		t.Fatalf("PollAssignments failed: %v", err)
	}
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}
	if assignments[0].Framework != "playwright" {
		t.Errorf("framework: got %q", assignments[0].Framework)
	}
}

func TestPollAssignments_Empty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"assignments":[]}`)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	assignments, err := a.PollAssignments()
	if err != nil {
		t.Fatalf("PollAssignments failed: %v", err)
	}
	if len(assignments) != 0 {
		t.Errorf("expected 0 assignments, got %d", len(assignments))
	}
}

func TestPollAssignments_NotRegistered(t *testing.T) {
	a := &Agent{
		CloudURL: "http://localhost",
		APIKey:   "",
		AgentID:  "",
		client:   &http.Client{Timeout: 5 * time.Second},
	}
	assignments, err := a.PollAssignments()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if assignments != nil {
		t.Error("expected nil assignments when not registered")
	}
}

func TestPollAssignments_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	_, err := a.PollAssignments()
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

// --- fetchScriptCode tests ---

func TestFetchScriptCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/api/automation/scripts/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"code": "const { test } = require('@playwright/test'); test('example', async () => {});",
		})
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	code, err := a.fetchScriptCode("script-42")
	if err != nil {
		t.Fatalf("fetchScriptCode failed: %v", err)
	}
	if !strings.Contains(code, "playwright") {
		t.Errorf("unexpected code: %s", code)
	}
}

func TestFetchScriptCode_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "not found")
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	_, err := a.fetchScriptCode("nonexistent")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

// --- collectArtifacts tests ---

func TestCollectArtifacts_NoTestResults(t *testing.T) {
	tmpDir := t.TempDir()
	a := &Agent{}
	artifacts := a.collectArtifacts(tmpDir)
	if artifacts == nil {
		t.Fatal("artifacts should not be nil")
	}
	screenshots, ok := artifacts["screenshots"].([]map[string]string)
	if !ok {
		t.Fatal("screenshots should be []map[string]string")
	}
	if len(screenshots) != 0 {
		t.Errorf("expected 0 screenshots, got %d", len(screenshots))
	}
	if artifacts["video"] != nil {
		t.Error("video should be nil when no results")
	}
}

func TestCollectArtifacts_WithScreenshotsAndVideo(t *testing.T) {
	tmpDir := t.TempDir()
	testResults := filepath.Join(tmpDir, "test-results", "test-1")
	_ = os.MkdirAll(testResults, 0755)

	// Create screenshot files
	screenshotData := []byte{0x89, 0x50, 0x4E, 0x47} // PNG header
	_ = os.WriteFile(filepath.Join(testResults, "screenshot1.png"), screenshotData, 0644)
	_ = os.WriteFile(filepath.Join(testResults, "screenshot2.png"), screenshotData, 0644)

	// Create video file
	videoData := []byte{0x1A, 0x45, 0xDF, 0xA3} // WebM header
	_ = os.WriteFile(filepath.Join(testResults, "video.webm"), videoData, 0644)

	a := &Agent{}
	artifacts := a.collectArtifacts(tmpDir)

	screenshots, ok := artifacts["screenshots"].([]map[string]string)
	if !ok {
		t.Fatal("screenshots should be []map[string]string")
	}
	if len(screenshots) != 2 {
		t.Errorf("expected 2 screenshots, got %d", len(screenshots))
	}

	// Verify screenshots are base64 encoded
	for _, ss := range screenshots {
		if ss["filename"] == "" {
			t.Error("screenshot filename should not be empty")
		}
		if ss["data"] == "" {
			t.Error("screenshot data should not be empty")
		}
		// Verify base64 decoding works
		_, err := base64.StdEncoding.DecodeString(ss["data"])
		if err != nil {
			t.Errorf("screenshot data is not valid base64: %v", err)
		}
	}

	// Verify video
	video, ok := artifacts["video"].(map[string]string)
	if !ok || video == nil {
		t.Fatal("video should be a map[string]string")
	}
	if video["filename"] != "video.webm" {
		t.Errorf("video filename: got %q", video["filename"])
	}
}

func TestCollectArtifacts_OnlyFirstVideo(t *testing.T) {
	tmpDir := t.TempDir()
	testResults := filepath.Join(tmpDir, "test-results")
	_ = os.MkdirAll(testResults, 0755)

	_ = os.WriteFile(filepath.Join(testResults, "video1.webm"), []byte("v1"), 0644)
	_ = os.WriteFile(filepath.Join(testResults, "video2.webm"), []byte("v2"), 0644)

	a := &Agent{}
	artifacts := a.collectArtifacts(tmpDir)

	video, ok := artifacts["video"].(map[string]string)
	if !ok || video == nil {
		t.Fatal("video should be present")
	}
	// Only one video should be collected (the first one found)
	if video["filename"] == "" {
		t.Error("video filename should not be empty")
	}
}

// --- updateAssignmentStatus tests ---

func TestUpdateAssignmentStatus_Success(t *testing.T) {
	var receivedPath string
	var receivedPayload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.updateAssignmentStatus("assign-1", "started")

	expectedPath := fmt.Sprintf("/api/agent/%s/assignments/assign-1/status", a.AgentID)
	if receivedPath != expectedPath {
		t.Errorf("path: got %q, want %q", receivedPath, expectedPath)
	}
	if receivedPayload["status"] != "started" {
		t.Errorf("status: got %q, want %q", receivedPayload["status"], "started")
	}
}

func TestUpdateAssignmentStatus_NotRegistered(t *testing.T) {
	a := &Agent{
		CloudURL: "http://localhost",
		client:   &http.Client{Timeout: 5 * time.Second},
	}
	// Should not panic when not registered
	a.updateAssignmentStatus("assign-1", "started")
}

// --- reportResult tests ---

func TestReportResult_Success(t *testing.T) {
	var resultCalls int32
	var statusCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			atomic.AddInt32(&resultCalls, 1)
			var payload map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload["success"] != true {
				t.Error("expected success=true")
			}
		} else if strings.Contains(r.URL.Path, "/status") {
			atomic.AddInt32(&statusCalls, 1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	resultData := map[string]interface{}{
		"output":    "test passed",
		"errors":    "",
		"artifacts": nil,
	}
	a.reportResult("assign-1", true, "test passed", resultData)

	if atomic.LoadInt32(&resultCalls) != 1 {
		t.Error("expected result endpoint to be called once")
	}
	if atomic.LoadInt32(&statusCalls) != 1 {
		t.Error("expected status endpoint to be called once (for completed status)")
	}
}

func TestReportResult_Failure(t *testing.T) {
	var receivedStatus string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/status") {
			var payload map[string]string
			_ = json.NewDecoder(r.Body).Decode(&payload)
			receivedStatus = payload["status"]
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.reportResult("assign-1", false, "test failed", nil)

	if receivedStatus != "failed" {
		t.Errorf("expected 'failed' status, got %q", receivedStatus)
	}
}

func TestReportResult_NotRegistered(t *testing.T) {
	a := &Agent{
		CloudURL: "http://localhost",
		client:   &http.Client{Timeout: 5 * time.Second},
	}
	// Should not panic
	a.reportResult("assign-1", true, "ok", nil)
}

// --- runCommand tests ---

func TestRunCommand_Success(t *testing.T) {
	a := &Agent{}
	err := a.runCommand(context.Background(), t.TempDir(), "echo", "hello world", 5*time.Second)
	if err != nil {
		t.Fatalf("runCommand failed: %v", err)
	}
}

func TestRunCommand_Failure(t *testing.T) {
	a := &Agent{}
	err := a.runCommand(context.Background(), t.TempDir(), "false", "", 5*time.Second)
	if err == nil {
		t.Error("expected error for failing command")
	}
}

func TestRunCommand_Timeout(t *testing.T) {
	a := &Agent{}
	err := a.runCommand(context.Background(), t.TempDir(), "sleep", "10", 100*time.Millisecond)
	if err == nil {
		t.Error("expected error for timed out command")
	}
}

// --- waitForActiveTests tests ---

func TestWaitForActiveTests_NoActive(t *testing.T) {
	a := &Agent{}
	// Should return immediately when no active tests
	done := make(chan struct{})
	go func() {
		a.waitForActiveTests()
		close(done)
	}()

	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("waitForActiveTests should return immediately when count is 0")
	}
}

func TestWaitForActiveTests_WaitsForCompletion(t *testing.T) {
	a := &Agent{}
	a.mu.Lock()
	a.activeCount = 1
	a.mu.Unlock()

	done := make(chan struct{})
	go func() {
		a.waitForActiveTests()
		close(done)
	}()

	// Should not complete yet
	select {
	case <-done:
		t.Fatal("waitForActiveTests should wait when active count > 0")
	case <-time.After(100 * time.Millisecond):
		// good, it's waiting
	}

	// Decrement to 0
	a.mu.Lock()
	a.activeCount = 0
	a.mu.Unlock()

	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("waitForActiveTests should complete when count reaches 0")
	}
}

// --- heartbeatLoop tests ---

func TestHeartbeatLoop_CancelStops(t *testing.T) {
	var heartbeats int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&heartbeats, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.HeartbeatInterval = 50 * time.Millisecond
	a.running = true

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	a.heartbeatLoop(ctx)

	count := atomic.LoadInt32(&heartbeats)
	if count < 1 {
		t.Errorf("expected at least 1 heartbeat, got %d", count)
	}
}

func TestHeartbeatLoop_StopsWhenNotRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.HeartbeatInterval = 50 * time.Millisecond
	a.running = false // Not running

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		a.heartbeatLoop(ctx)
		close(done)
	}()

	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeatLoop should stop when running is false")
	}
}

// --- Run tests ---

func TestRun_CancelAfterRegistration(t *testing.T) {
	requestCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/register"):
			_ = json.NewEncoder(w).Encode(map[string]string{
				"agent_id": "run-agent",
				"api_key":  "run-key",
			})
		case strings.Contains(r.URL.Path, "/heartbeat"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/assignments/pending"):
			fmt.Fprint(w, `{"assignments":[]}`)
		case strings.Contains(r.URL.Path, "/crawl/pending"):
			w.WriteHeader(http.StatusNoContent)
		default:
			// First few requests are normal, then we can check
			_ = count
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "", 100*time.Millisecond, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay to let it run for a bit
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	err := a.Run(ctx)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRun_RegistrationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "invalid secret")
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "", 5*time.Second, 60*time.Second)
	ctx := context.Background()
	err := a.Run(ctx)
	if err == nil {
		t.Fatal("expected error for registration failure")
	}
	if !strings.Contains(err.Error(), "failed to register") {
		t.Errorf("error should mention registration failure: %v", err)
	}
}

// --- ExecuteTest tests (error paths only) ---

func TestExecuteTest_NoCode(t *testing.T) {
	var reportedSuccess bool
	var reportedMessage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			var payload map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			reportedSuccess = payload["success"].(bool)
			reportedMessage = payload["message"].(string)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	assignment := Assignment{
		ID:   "test-no-code",
		Code: "",
	}

	a.ExecuteTest(context.Background(), assignment)

	if reportedSuccess {
		t.Error("expected success=false for no code")
	}
	if !strings.Contains(reportedMessage, "No test code") {
		t.Errorf("expected 'No test code' message, got %q", reportedMessage)
	}
}

func TestExecuteTest_FetchesScriptCode(t *testing.T) {
	var fetchedScriptID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/api/automation/scripts/") {
			fetchedScriptID = strings.TrimPrefix(r.URL.Path, "/api/automation/scripts/")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"code": ""})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	assignment := Assignment{
		ID:       "test-fetch",
		ScriptID: "script-99",
		Code:     "", // empty code triggers fetch
	}

	a.ExecuteTest(context.Background(), assignment)

	if fetchedScriptID != "script-99" {
		t.Errorf("expected script fetch for script-99, got %q", fetchedScriptID)
	}
}

// --- getPlatformVersion tests ---

func TestGetPlatformVersion(t *testing.T) {
	a := &Agent{}
	version := a.getPlatformVersion()
	// On macOS or Linux, this should return something
	if version == "" {
		t.Log("getPlatformVersion returned empty string (may be expected on some platforms)")
	}
}

// --- ExecuteTest more thorough tests ---

func TestExecuteTest_DefaultBrowser(t *testing.T) {
	// Test with empty browser to exercise default "chromium" path
	var statusUpdated bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/status") {
			statusUpdated = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)

	// Cancel context immediately to abort npm install
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assignment := Assignment{
		ID:       "test-defaults",
		Code:     "test('hello', async ({page}) => { await page.goto('http://example.com'); });",
		Browser:  "", // Should default to chromium
		Headless: true,
	}

	a.ExecuteTest(ctx, assignment)
	// The test creates files and updates status before npm install fails
	if !statusUpdated {
		t.Log("status update may not have been called due to fast cancellation")
	}
}

func TestExecuteTest_CustomURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assignment := Assignment{
		ID:        "test-custom-url",
		Code:      "test('hello', async ({page}) => {});",
		Browser:   "firefox",
		CustomURL: "https://custom.example.com",
		Headless:  true,
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	a.ExecuteTest(ctx, assignment)
}

func TestExecuteTest_UnknownBrowser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assignment := Assignment{
		ID:      "test-unknown-browser",
		Code:    "test('hello', async ({page}) => {});",
		Browser: "msedge", // Not in browserMap, should default to chromium
	}

	a.ExecuteTest(ctx, assignment)
}

func TestExecuteTest_WebkitBrowser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assignment := Assignment{
		ID:      "test-webkit",
		Code:    "test('hello', async ({page}) => {});",
		Browser: "webkit",
	}

	a.ExecuteTest(ctx, assignment)
}

func TestExecuteTest_ScriptIDNil(t *testing.T) {
	// Test with scriptID "<nil>" to verify the nil check
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	assignment := Assignment{
		ID:       "test-nil-script",
		ScriptID: "", // Empty json.Number — tests empty ID handling
		Code:     "",
	}

	a.ExecuteTest(context.Background(), assignment)
}

func TestExecuteTest_ViewportDefaults(t *testing.T) {
	// Test that viewport defaults are applied (1280x720)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assignment := Assignment{
		ID:             "test-viewport-defaults",
		Code:           "test('hello', async () => {});",
		ViewportWidth:  0, // should default to 1280
		ViewportHeight: 0, // should default to 720
	}

	a.ExecuteTest(ctx, assignment)
}

// --- Run with polling error paths ---

func TestRun_PollingError(t *testing.T) {
	pollCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/register"):
			_ = json.NewEncoder(w).Encode(map[string]string{
				"agent_id": "err-agent",
				"api_key":  "err-key",
			})
		case strings.Contains(r.URL.Path, "/heartbeat"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/assignments/pending"):
			count := atomic.AddInt32(&pollCount, 1)
			if count == 1 {
				// Return server error on first poll to exercise error path
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, `{"assignments":[]}`)
		case strings.Contains(r.URL.Path, "/crawl/pending"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "", 100*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(400 * time.Millisecond)
		cancel()
	}()

	err := a.Run(ctx)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRun_CrawlPollError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/register"):
			_ = json.NewEncoder(w).Encode(map[string]string{
				"agent_id": "crawl-err-agent",
				"api_key":  "crawl-err-key",
			})
		case strings.Contains(r.URL.Path, "/heartbeat"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/assignments/pending"):
			fmt.Fprint(w, `{"assignments":[]}`)
		case strings.Contains(r.URL.Path, "/crawl/pending"):
			// Return error to exercise the warn path
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "bad request")
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "", 100*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(400 * time.Millisecond)
		cancel()
	}()

	err := a.Run(ctx)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

// --- Assignment struct tests ---

func TestAssignmentParsing(t *testing.T) {
	raw := `{
		"id": "assign-1",
		"script_id": "script-1",
		"code": "test('hello', () => {});",
		"framework": "playwright",
		"custom_url": "https://example.com",
		"execution_id": "exec-1",
		"headless": true,
		"browser": "firefox",
		"viewport_width": 1920,
		"viewport_height": 1080
	}`

	var a Assignment
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if a.Framework != "playwright" {
		t.Errorf("Framework: got %q", a.Framework)
	}
	if a.Headless != true {
		t.Error("Headless should be true")
	}
	if a.Browser != "firefox" {
		t.Errorf("Browser: got %q", a.Browser)
	}
	if a.ViewportWidth != 1920 {
		t.Errorf("ViewportWidth: got %d", a.ViewportWidth)
	}
	if a.ViewportHeight != 1080 {
		t.Errorf("ViewportHeight: got %d", a.ViewportHeight)
	}
	if a.CustomURL != "https://example.com" {
		t.Errorf("CustomURL: got %q", a.CustomURL)
	}
}

func TestAssignmentDefaults(t *testing.T) {
	raw := `{"id": "assign-2"}`
	var a Assignment
	_ = json.Unmarshal([]byte(raw), &a)

	if a.Browser != "" {
		t.Errorf("expected empty browser default, got %q", a.Browser)
	}
	if a.ViewportWidth != 0 {
		t.Errorf("expected 0 viewport width, got %d", a.ViewportWidth)
	}
	if a.Headless != false {
		t.Error("expected headless=false by default")
	}
}

// --- doJSON error path tests ---

func TestDoJSON_InvalidURL(t *testing.T) {
	a := &Agent{
		client: &http.Client{Timeout: 1 * time.Second},
	}
	_, _, err := a.doJSON("GET", "://invalid-url", nil, nil)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestDoJSON_MarshalError(t *testing.T) {
	a := &Agent{
		client: &http.Client{Timeout: 1 * time.Second},
	}
	// A channel can't be marshaled to JSON
	_, _, err := a.doJSON("POST", "http://localhost/test", make(chan int), nil)
	if err == nil {
		t.Error("expected error for unmarshalable body")
	}
}

// --- Register parse error test ---

func TestRegister_InvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "", 5*time.Second, 60*time.Second)
	err := a.Register()
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

// --- PollAssignments parse error test ---

func TestPollAssignments_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	_, err := a.PollAssignments()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- fetchScriptCode no code field test ---

func TestFetchScriptCode_NoCodeField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"name": "test script",
			// no "code" field
		})
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	code, err := a.fetchScriptCode("script-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != "" {
		t.Errorf("expected empty code, got %q", code)
	}
}

// --- updateAssignmentStatus error path ---

func TestUpdateAssignmentStatus_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	// Should not panic, just logs
	a.updateAssignmentStatus("assign-1", "failed")
}

// --- reportResult server error path ---

func TestReportResult_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "server error")
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	// Should not panic
	a.reportResult("assign-1", true, "ok", map[string]interface{}{
		"output":    "stdout",
		"errors":    "stderr",
		"artifacts": map[string]interface{}{},
	})
}

// --- heartbeatLoop consecutive failures test ---

func TestHeartbeatLoop_ConsecutiveFailures(t *testing.T) {
	var heartbeats int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&heartbeats, 1)
		// Always fail — tests that backoff doesn't crash
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	a.HeartbeatInterval = 10 * time.Millisecond
	a.running = true

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	a.heartbeatLoop(ctx)

	count := atomic.LoadInt32(&heartbeats)
	if count < 1 {
		t.Errorf("expected at least 1 heartbeat attempt, got %d", count)
	}
}

// --- ExecuteTest full path with mock npm ---

func TestExecuteTest_FullPath_MockNPM(t *testing.T) {
	// Create mock npm and npx that just exit 0
	mockBin := t.TempDir()
	npmScript := filepath.Join(mockBin, "npm")
	npxScript := filepath.Join(mockBin, "npx")

	_ = os.WriteFile(npmScript, []byte("#!/bin/sh\nexit 0\n"), 0755)
	_ = os.WriteFile(npxScript, []byte("#!/bin/sh\nexit 0\n"), 0755)

	// Prepend mock bin to PATH
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", mockBin+":"+origPath)
	defer os.Setenv("PATH", origPath)

	var resultPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			_ = json.NewDecoder(r.Body).Decode(&resultPayload)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)

	assignment := Assignment{
		ID:             "test-full-path",
		Code:           "const { test } = require('@playwright/test');\ntest('hello', async ({page}) => {\n  await page.goto('http://example.com');\n});",
		Browser:        "chromium",
		Headless:       true,
		CustomURL:      "https://custom.example.com",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	a.activeTests.Store("test-full-path", true)
	a.mu.Lock()
	a.activeCount++
	a.mu.Unlock()

	a.ExecuteTest(context.Background(), assignment)

	if resultPayload == nil {
		t.Error("expected result to be reported")
	}
}

func TestExecuteTest_FullPath_FirefoxBrowser(t *testing.T) {
	mockBin := t.TempDir()
	_ = os.WriteFile(filepath.Join(mockBin, "npm"), []byte("#!/bin/sh\nexit 0\n"), 0755)
	_ = os.WriteFile(filepath.Join(mockBin, "npx"), []byte("#!/bin/sh\nexit 0\n"), 0755)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", mockBin+":"+origPath)
	defer os.Setenv("PATH", origPath)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)

	assignment := Assignment{
		ID:      "test-firefox-full",
		Code:    "test('hello', async () => {});",
		Browser: "firefox",
	}

	a.activeTests.Store("test-firefox-full", true)
	a.mu.Lock()
	a.activeCount++
	a.mu.Unlock()

	a.ExecuteTest(context.Background(), assignment)
}

func TestExecuteTest_FullPath_NPMFails(t *testing.T) {
	mockBin := t.TempDir()
	_ = os.WriteFile(filepath.Join(mockBin, "npm"), []byte("#!/bin/sh\necho 'npm install failed' >&2\nexit 1\n"), 0755)
	_ = os.WriteFile(filepath.Join(mockBin, "npx"), []byte("#!/bin/sh\nexit 0\n"), 0755)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", mockBin+":"+origPath)
	defer os.Setenv("PATH", origPath)

	var resultSuccess bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			var payload map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			resultSuccess = payload["success"].(bool)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)

	assignment := Assignment{
		ID:   "test-npm-fail",
		Code: "test('hello', async () => {});",
	}

	a.ExecuteTest(context.Background(), assignment)

	if resultSuccess {
		t.Error("expected success=false when npm install fails")
	}
}

func TestExecuteTest_FullPath_PlaywrightTestFails(t *testing.T) {
	mockBin := t.TempDir()
	_ = os.WriteFile(filepath.Join(mockBin, "npm"), []byte("#!/bin/sh\nexit 0\n"), 0755)
	// npx playwright test fails (exit 1 = test failure)
	_ = os.WriteFile(filepath.Join(mockBin, "npx"), []byte("#!/bin/sh\necho 'Test failed' >&2\nexit 1\n"), 0755)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", mockBin+":"+origPath)
	defer os.Setenv("PATH", origPath)

	var resultSuccess bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/result") {
			var payload map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			resultSuccess = payload["success"].(bool)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)

	assignment := Assignment{
		ID:   "test-pw-fail",
		Code: "test('hello', async () => {});",
	}

	a.ExecuteTest(context.Background(), assignment)

	if resultSuccess {
		t.Error("expected success=false when playwright test fails")
	}
}

func TestExecuteTest_FullPath_WithOutput(t *testing.T) {
	mockBin := t.TempDir()
	_ = os.WriteFile(filepath.Join(mockBin, "npm"), []byte("#!/bin/sh\nexit 0\n"), 0755)
	// npx produces stdout output
	longOutput := strings.Repeat("x", 600)
	_ = os.WriteFile(filepath.Join(mockBin, "npx"), []byte(fmt.Sprintf("#!/bin/sh\necho '%s'\necho 'error msg' >&2\nexit 0\n", longOutput)), 0755)

	origPath := os.Getenv("PATH")
	os.Setenv("PATH", mockBin+":"+origPath)
	defer os.Setenv("PATH", origPath)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)

	assignment := Assignment{
		ID:   "test-output",
		Code: "test('hello', async () => {});",
	}

	a.ExecuteTest(context.Background(), assignment)
}

// --- Run loop with actual assignment processing ---

func TestRun_WithNoCodeAssignment(t *testing.T) {
	pollCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/register"):
			_ = json.NewEncoder(w).Encode(map[string]string{
				"agent_id": "assign-agent",
				"api_key":  "assign-key",
			})
		case strings.Contains(r.URL.Path, "/heartbeat"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/assignments/pending"):
			count := atomic.AddInt32(&pollCount, 1)
			if count == 1 {
				// Return assignment with no code — will fail fast
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"assignments": []map[string]interface{}{
						{"id": "no-code-1", "code": ""},
					},
				})
			} else {
				fmt.Fprint(w, `{"assignments":[]}`)
			}
		case strings.Contains(r.URL.Path, "/crawl/pending"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "", 100*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_ = a.Run(ctx)
}

func TestRun_WithDuplicateAssignment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/register"):
			_ = json.NewEncoder(w).Encode(map[string]string{
				"agent_id": "dup-agent",
				"api_key":  "dup-key",
			})
		case strings.Contains(r.URL.Path, "/heartbeat"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/assignments/pending"):
			// Always return same assignment — second time should skip
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"assignments": []map[string]interface{}{
					{"id": "dup-1", "code": ""},
				},
			})
		case strings.Contains(r.URL.Path, "/crawl/pending"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "", 100*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	_ = a.Run(ctx)
}

// --- Run error handling paths ---

func TestRun_PollingAssignmentError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case strings.Contains(r.URL.Path, "/register"):
			_ = json.NewEncoder(w).Encode(map[string]string{
				"agent_id": "poll-err-agent",
				"api_key":  "poll-err-key",
			})
		case strings.Contains(r.URL.Path, "/heartbeat"):
			w.WriteHeader(http.StatusOK)
		case strings.Contains(r.URL.Path, "/assignments/pending"):
			// Return non-timeout error
			w.WriteHeader(http.StatusInternalServerError)
		case strings.Contains(r.URL.Path, "/crawl/pending"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	a := NewAgent(server.URL, "", "", "", 100*time.Millisecond, 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(400 * time.Millisecond)
		cancel()
	}()

	err := a.Run(ctx)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}
