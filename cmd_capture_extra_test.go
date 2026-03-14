package main

import (
	"testing"

	"github.com/chromedp/cdproto/network"
)

func TestBuildPlaywrightStorageState_WithRealCookies(t *testing.T) {
	cookies := []*network.Cookie{
		{
			Name:     "session",
			Value:    "abc123",
			Domain:   ".example.com",
			Path:     "/",
			Expires:  1700000000,
			HTTPOnly: true,
			Secure:   true,
			SameSite: network.CookieSameSiteLax,
		},
		{
			Name:     "prefs",
			Value:    "dark",
			Domain:   "example.com",
			Path:     "/app",
			Expires:  0,
			HTTPOnly: false,
			Secure:   false,
			SameSite: network.CookieSameSiteStrict,
		},
		{
			Name:     "tracker",
			Value:    "xyz",
			Domain:   ".ads.com",
			Path:     "/",
			SameSite: network.CookieSameSiteNone,
		},
	}

	state := buildPlaywrightStorageState(cookies, "https://example.com/page", nil)

	if len(state.Cookies) != 3 {
		t.Fatalf("expected 3 cookies, got %d", len(state.Cookies))
	}

	// Check first cookie
	c := state.Cookies[0]
	if c.Name != "session" {
		t.Errorf("cookie 0 name: got %q", c.Name)
	}
	if c.SameSite != "Lax" {
		t.Errorf("cookie 0 sameSite: got %q, want 'Lax'", c.SameSite)
	}
	if !c.HTTPOnly {
		t.Error("cookie 0 should be HTTPOnly")
	}
	if !c.Secure {
		t.Error("cookie 0 should be Secure")
	}

	// Check Strict cookie
	c1 := state.Cookies[1]
	if c1.SameSite != "Strict" {
		t.Errorf("cookie 1 sameSite: got %q, want 'Strict'", c1.SameSite)
	}

	// Check None cookie
	c2 := state.Cookies[2]
	if c2.SameSite != "None" {
		t.Errorf("cookie 2 sameSite: got %q, want 'None'", c2.SameSite)
	}

	// No origins without localStorage
	if len(state.Origins) != 0 {
		t.Errorf("expected 0 origins without localStorage, got %d", len(state.Origins))
	}
}

func TestBuildPlaywrightStorageState_CookiesAndLocalStorage(t *testing.T) {
	cookies := []*network.Cookie{
		{
			Name:   "auth",
			Value:  "token123",
			Domain: ".example.com",
			Path:   "/",
		},
	}
	localStorage := []map[string]string{
		{"name": "theme", "value": "dark"},
		{"name": "lang", "value": "en"},
	}

	state := buildPlaywrightStorageState(cookies, "https://example.com/app/dashboard", localStorage)

	if len(state.Cookies) != 1 {
		t.Errorf("expected 1 cookie, got %d", len(state.Cookies))
	}
	if len(state.Origins) != 1 {
		t.Fatalf("expected 1 origin, got %d", len(state.Origins))
	}

	origin := state.Origins[0]["origin"]
	if origin != "https://example.com" {
		t.Errorf("origin: got %v, want 'https://example.com'", origin)
	}
}

func TestBuildPlaywrightStorageState_URLWithoutPath(t *testing.T) {
	localStorage := []map[string]string{
		{"name": "key", "value": "val"},
	}

	state := buildPlaywrightStorageState(nil, "https://example.com", localStorage)

	if len(state.Origins) != 1 {
		t.Fatalf("expected 1 origin, got %d", len(state.Origins))
	}
	origin := state.Origins[0]["origin"]
	if origin != "https://example.com" {
		t.Errorf("origin: got %v, want 'https://example.com'", origin)
	}
}
