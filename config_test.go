package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_MissingFile(t *testing.T) {
	// Override home to a temp dir
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig returned error for missing file: %v", err)
	}
	if cfg.Token != "" || cfg.APIURL != "" || cfg.AgentID != "" {
		t.Fatalf("expected zero config, got %+v", cfg)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	original := &Config{
		Token:              "test-token-123",
		APIURL:             "https://app.qualitymax.io",
		AgentID:            "agent-uuid",
		APIKey:             "hex-api-key",
		RegistrationSecret: "secret",
	}

	if err := original.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file permissions
	path := filepath.Join(tmp, ".qamax", "config.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("expected 0600 permissions, got %o", perm)
	}

	// Verify directory permissions
	dirInfo, err := os.Stat(filepath.Join(tmp, ".qamax"))
	if err != nil {
		t.Fatalf("config dir not created: %v", err)
	}
	dirPerm := dirInfo.Mode().Perm()
	if dirPerm != 0700 {
		t.Errorf("expected 0700 dir permissions, got %o", dirPerm)
	}

	// Load and compare
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loaded.Token != original.Token {
		t.Errorf("Token: got %q, want %q", loaded.Token, original.Token)
	}
	if loaded.APIURL != original.APIURL {
		t.Errorf("APIURL: got %q, want %q", loaded.APIURL, original.APIURL)
	}
	if loaded.AgentID != original.AgentID {
		t.Errorf("AgentID: got %q, want %q", loaded.AgentID, original.AgentID)
	}
	if loaded.APIKey != original.APIKey {
		t.Errorf("APIKey: got %q, want %q", loaded.APIKey, original.APIKey)
	}
	if loaded.RegistrationSecret != original.RegistrationSecret {
		t.Errorf("RegistrationSecret: got %q, want %q", loaded.RegistrationSecret, original.RegistrationSecret)
	}
}

func TestSaveConfig_JSONFormat(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &Config{
		Token:  "tok",
		APIURL: "https://example.com",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	path := filepath.Join(tmp, ".qamax", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if raw["token"] != "tok" {
		t.Errorf("expected token=tok, got %v", raw["token"])
	}
}

func TestGetAPIBaseURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://app.qualitymax.io", "https://app.qualitymax.io"},
		{"https://app.qualitymax.io/", "https://app.qualitymax.io"},
		{"https://app.qualitymax.io/app", "https://app.qualitymax.io"},
		{"https://app.qualitymax.io/app/", "https://app.qualitymax.io"},
		{"", ""},
	}

	for _, tt := range tests {
		cfg := &Config{APIURL: tt.input}
		got := cfg.GetAPIBaseURL()
		if got != tt.want {
			t.Errorf("GetAPIBaseURL(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	dir := filepath.Join(tmp, ".qamax")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("not json"), 0600)

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
