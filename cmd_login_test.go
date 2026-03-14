package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestLoginCallbackHandler(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	tokenCh := make(chan string, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Missing token parameter", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body>Success</body></html>")
		tokenCh <- token
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Simulate browser callback
	resp, err := http.Get(server.URL + "/callback?token=test-jwt-token")
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	token := <-tokenCh
	if token != "test-jwt-token" {
		t.Errorf("expected token 'test-jwt-token', got %q", token)
	}

	// Verify saving token to config works
	cfg := &Config{Token: token, APIURL: "https://app.qualitymax.io"}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if loaded.Token != "test-jwt-token" {
		t.Errorf("loaded token %q, want 'test-jwt-token'", loaded.Token)
	}
}

func TestLoginCallbackMissingToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "Missing token parameter", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/callback")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestLoginSavesConfigFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cfg := &Config{
		Token:  "saved-token",
		APIURL: "https://app.qualitymax.io",
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	path := filepath.Join(tmp, ".qamax", "config.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}
}
