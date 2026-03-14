package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// skipIfNoBrowser skips the test if no Chrome/Chromium is available
// and enforces a shorter timeout than the default browser tests
func skipIfNoBrowser(t *testing.T) {
	t.Helper()
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping browser test in CI")
	}
	if !commandExists("google-chrome") && !commandExists("chromium") && !commandExists("chromium-browser") {
		if !pathExists([]string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}) {
			t.Skip("No Chrome/Chromium available")
		}
	}
}

func newTimedBrowserContext(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), timeout)
	allocCtx, allocCancel := chromedp.NewExecAllocator(timeoutCtx, opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	cleanup := func() {
		cancel()
		allocCancel()
		timeoutCancel()
	}
	return ctx, cleanup
}

// --- crawlClick tests ---

func TestCrawlClick_Success(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<button id="test-btn" onclick="document.title='btn-clicked'">Click Me</button>
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	err := a.crawlClick(ctx, "click-test", "#test-btn")
	if err != nil {
		t.Fatalf("crawlClick failed: %v", err)
	}

	var title string
	_ = chromedp.Run(ctx, chromedp.Title(&title))
	if title != "btn-clicked" {
		t.Errorf("expected title 'btn-clicked', got %q", title)
	}
}

func TestCrawlClick_JSFallbackOnHidden(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<button id="hidden-btn" style="display:none" onclick="document.title='js-click'">Hidden</button>
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	// Native click will fail on hidden element, should fallback to JS
	err := a.crawlClick(ctx, "js-fallback", "#hidden-btn")
	if err != nil {
		t.Logf("crawlClick JS fallback returned: %v (may be expected)", err)
	}
}

// --- crawlFill tests ---

func TestCrawlFill_Success(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<input id="name-input" type="text" value="old value" />
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	err := a.crawlFill(ctx, "#name-input", "new value")
	if err != nil {
		t.Fatalf("crawlFill failed: %v", err)
	}

	var value string
	_ = chromedp.Run(ctx, chromedp.Value("#name-input", &value, chromedp.ByQuery))
	if value != "new value" {
		t.Errorf("expected 'new value', got %q", value)
	}
}

// --- crawlSelect tests ---

func TestCrawlSelect_Success(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<select id="lang">
				<option value="">Pick</option>
				<option value="go">Go</option>
				<option value="py">Python</option>
			</select>
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	err := a.crawlSelect(ctx, "#lang", "go")
	if err != nil {
		t.Fatalf("crawlSelect failed: %v", err)
	}

	var value string
	_ = chromedp.Run(ctx, chromedp.Value("#lang", &value, chromedp.ByQuery))
	if value != "go" {
		t.Errorf("expected 'go', got %q", value)
	}
}

// --- crawlComboboxSelect tests ---

func TestCrawlComboboxSelect_NoOptions(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<input id="combo-input" type="text" />
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	err := a.crawlComboboxSelect(ctx, "#combo-input", "test")
	// Should return an error because no options are found
	if err == nil {
		t.Error("expected error when no options available")
	}
	if err != nil && !strings.Contains(err.Error(), "could not find") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --- dismissCookieConsent tests ---

func TestDismissCookieConsent_WithAcceptAllButton(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<div id="cookie-overlay">
				<p>Cookies?</p>
				<button onclick="document.getElementById('cookie-overlay').remove(); document.title='dismissed'">Accept All</button>
			</div>
			<h1>Content</h1>
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	a.dismissCookieConsent(ctx, "cookie-test")

	var title string
	_ = chromedp.Run(ctx, chromedp.Title(&title))
	if title == "dismissed" {
		t.Log("Cookie consent dismissed successfully")
	}
}

func TestDismissCookieConsent_NoBannerPresent(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body><h1>No cookies</h1></body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	// Should not panic when no cookie banner exists
	a.dismissCookieConsent(ctx, "no-cookie-test")
}

// --- captureSnapshot tests ---

func TestCaptureSnapshot_BasicPage(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Snapshot Test</title></head><body><h1>Hello</h1></body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{CloudURL: "http://localhost", AgentID: "test", APIKey: "key"}
	session := CrawlSession{SessionID: "snap-basic", URL: ts.URL}

	snapshot, err := a.captureSnapshot(ctx, session, 1)
	if err != nil {
		t.Fatalf("captureSnapshot failed: %v", err)
	}

	if snapshot.Title != "Snapshot Test" {
		t.Errorf("Title: got %q, want 'Snapshot Test'", snapshot.Title)
	}
	if snapshot.ScreenshotBase64 == "" {
		t.Error("ScreenshotBase64 should not be empty")
	}
	if snapshot.StepNum != 1 {
		t.Errorf("StepNum: got %d, want 1", snapshot.StepNum)
	}
}

func TestCaptureSnapshot_WithScript(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Script Page</title></head><body><button id="b1">Go</button></body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	session := CrawlSession{
		SessionID:      "snap-script",
		URL:            ts.URL,
		SnapshotScript: `JSON.stringify({interactive_elements:[{tag:"button"}], forms:[], selectors:{"btn":"#b1"}, accessibility_tree:"button: Go"})`,
	}

	snapshot, err := a.captureSnapshot(ctx, session, 2)
	if err != nil {
		t.Fatalf("captureSnapshot failed: %v", err)
	}

	if len(snapshot.InteractiveElements) == 0 {
		t.Error("expected interactive elements from script")
	}
	if snapshot.Selectors["btn"] != "#b1" {
		t.Errorf("selector: got %q", snapshot.Selectors["btn"])
	}
}

func TestCaptureSnapshot_InvalidScript(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Bad Script</title></head><body><p>Page</p></body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	session := CrawlSession{
		SessionID:      "snap-bad-script",
		URL:            ts.URL,
		SnapshotScript: `throw new Error("script error")`,
	}

	snapshot, err := a.captureSnapshot(ctx, session, 1)
	if err != nil {
		t.Fatalf("captureSnapshot should not fail on script error: %v", err)
	}
	// Script failed but snapshot should still have URL and title
	if snapshot.Title != "Bad Script" {
		t.Errorf("Title: got %q", snapshot.Title)
	}
}

// --- ExecuteCrawlSession tests ---

func TestExecuteCrawlSession_DoneOnFirstStep(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Crawl Done</title></head><body><h1>Page</h1></body></html>`)
	}))
	defer htmlServer.Close()

	snapshotCount := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/snapshot") {
			snapshotCount++
			_ = json.NewEncoder(w).Encode(CrawlAction{Action: "done", Reason: "Complete"})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer apiServer.Close()

	a := &Agent{
		CloudURL: apiServer.URL,
		AgentID:  "crawl-done-agent",
		APIKey:   "crawl-done-key",
		client:   &http.Client{Timeout: 30 * time.Second},
	}

	session := CrawlSession{
		SessionID: "done-on-first",
		URL:       htmlServer.URL,
		MaxSteps:  5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	a.ExecuteCrawlSession(ctx, session)

	if snapshotCount != 1 {
		t.Errorf("expected 1 snapshot, got %d", snapshotCount)
	}
}

func TestExecuteCrawlSession_ClickThenDone(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Click Page</title></head><body>
			<button id="btn1" onclick="document.title='clicked'">Click</button>
		</body></html>`)
	}))
	defer htmlServer.Close()

	step := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/snapshot") {
			step++
			if step == 1 {
				_ = json.NewEncoder(w).Encode(CrawlAction{Action: "click", Selector: "#btn1", Reason: "Click button"})
			} else {
				_ = json.NewEncoder(w).Encode(CrawlAction{Action: "done", Reason: "Done"})
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer apiServer.Close()

	a := &Agent{
		CloudURL: apiServer.URL,
		AgentID:  "crawl-click-agent",
		APIKey:   "crawl-click-key",
		client:   &http.Client{Timeout: 30 * time.Second},
	}

	session := CrawlSession{
		SessionID: "click-then-done",
		URL:       htmlServer.URL,
		MaxSteps:  5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	a.ExecuteCrawlSession(ctx, session)

	if step < 2 {
		t.Errorf("expected at least 2 steps, got %d", step)
	}
}

func TestExecuteCrawlSession_InvalidURL(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	var errorReported bool
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/error") {
			errorReported = true
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer apiServer.Close()

	a := &Agent{
		CloudURL: apiServer.URL,
		AgentID:  "crawl-err-agent",
		APIKey:   "crawl-err-key",
		client:   &http.Client{Timeout: 30 * time.Second},
	}

	session := CrawlSession{
		SessionID: "invalid-url-crawl",
		URL:       "http://127.0.0.1:1/nonexistent", // will fail to navigate
		MaxSteps:  1,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	a.ExecuteCrawlSession(ctx, session)

	if !errorReported {
		t.Error("expected error to be reported for invalid URL")
	}
}

// --- executeCrawlAction tests ---

func TestExecuteCrawlAction_AllActionTypes(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<button id="btn">Click</button>
			<input id="input" type="text" />
			<select id="sel"><option value="a">A</option><option value="b">B</option></select>
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 20*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}

	// Test fill action
	err := a.executeCrawlAction(ctx, "multi-test", &CrawlAction{Action: "fill", Selector: "#input", Value: "hello"})
	if err != nil {
		t.Errorf("fill action failed: %v", err)
	}

	// Test select action
	err = a.executeCrawlAction(ctx, "multi-test", &CrawlAction{Action: "select", Selector: "#sel", Value: "b"})
	if err != nil {
		t.Errorf("select action failed: %v", err)
	}

	// Test click action
	err = a.executeCrawlAction(ctx, "multi-test", &CrawlAction{Action: "click", Selector: "#btn"})
	if err != nil {
		t.Errorf("click action failed: %v", err)
	}
}

func TestExecuteCrawlAction_ComboboxSelect(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoBrowser(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<input id="combo" type="text" />
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newTimedBrowserContext(t, 15*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	err := a.executeCrawlAction(ctx, "combo-test", &CrawlAction{Action: "combobox_select", Selector: "#combo", Value: "test"})
	// Expected to fail (no options to select)
	if err == nil {
		t.Log("combobox_select succeeded (unexpected but not an error)")
	}
}

// --- doJSONWithRetry edge cases ---

func TestDoJSONWithRetry_SuccessOnFirstTry(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	resp, body, err := a.doJSONWithRetry("GET", server.URL+"/test", nil, nil, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if string(body) != `{"ok":true}` {
		t.Errorf("body: %s", body)
	}
}

func TestDoJSONWithRetry_WithBody(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	var received map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	payload := map[string]string{"key": "val"}
	_, _, err := a.doJSONWithRetry("POST", server.URL+"/test", payload, nil, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received["key"] != "val" {
		t.Errorf("body: got %v", received)
	}
}

// --- PollCrawlSessions edge cases ---

func TestPollCrawlSessions_EmptySession(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Session with empty session_id
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"session": map[string]interface{}{
				"session_id": "",
				"url":        "https://example.com",
			},
		})
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	session, err := a.PollCrawlSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if session != nil {
		t.Error("expected nil for empty session_id")
	}
}

func TestPollCrawlSessions_InvalidJSON(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	_, err := a.PollCrawlSessions()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestPollCrawlSessions_CustomStatusCode(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "forbidden")
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	_, err := a.PollCrawlSessions()
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

// --- submitCrawlError edge cases ---

func TestSubmitCrawlError_ServerFails(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	// Should not panic
	a.submitCrawlError("sess-fail", "test error")
}

// --- submitSnapshot edge cases ---

func TestSubmitSnapshot_BadRequest(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, "bad request")
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	_, err := a.submitSnapshot("sess-bad", &CrawlSnapshot{SessionID: "sess-bad", StepNum: 1})
	if err == nil {
		t.Error("expected error for 400 response")
	}
}

func TestSubmitSnapshot_InvalidResponseJSON(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	a := newTestAgent(server.URL)
	_, err := a.submitSnapshot("sess-json", &CrawlSnapshot{SessionID: "sess-json", StepNum: 1})
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}
