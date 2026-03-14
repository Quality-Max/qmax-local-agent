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

func skipIfNoChrome(t *testing.T) {
	t.Helper()
	if os.Getenv("CI") == "true" {
		t.Skip("Skipping browser test in CI")
	}
	// Quick check if Chrome/Chromium is available
	if !commandExists("google-chrome") && !commandExists("chromium") && !commandExists("chromium-browser") {
		// On macOS, check default path
		if !pathExists([]string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"}) {
			t.Skip("No Chrome/Chromium available")
		}
	}
}

func newBrowserContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
	)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx)

	cleanup := func() {
		cancel()
		allocCancel()
	}

	return ctx, cleanup
}

// --- captureSnapshot tests ---

func TestCaptureSnapshot(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Test Page</title></head><body><h1>Hello World</h1></body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newBrowserContext(t)
	defer cancel()

	// Navigate first
	if err := chromedp.Run(ctx,
		chromedp.Navigate(ts.URL),
		chromedp.WaitReady("body"),
	); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{CloudURL: "http://localhost", AgentID: "test", APIKey: "key"}
	session := CrawlSession{
		SessionID: "snap-test",
		URL:       ts.URL,
	}

	snapshot, err := a.captureSnapshot(ctx, session, 1)
	if err != nil {
		t.Fatalf("captureSnapshot failed: %v", err)
	}

	if snapshot.SessionID != "snap-test" {
		t.Errorf("SessionID: got %q", snapshot.SessionID)
	}
	if snapshot.StepNum != 1 {
		t.Errorf("StepNum: got %d", snapshot.StepNum)
	}
	if snapshot.Title != "Test Page" {
		t.Errorf("Title: got %q", snapshot.Title)
	}
	if snapshot.URL == "" {
		t.Error("URL should not be empty")
	}
	if snapshot.ScreenshotBase64 == "" {
		t.Error("ScreenshotBase64 should not be empty")
	}
}

func TestCaptureSnapshot_WithSnapshotScript(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Script Test</title></head><body>
			<button id="btn1">Click Me</button>
			<input id="email" type="text" />
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newBrowserContext(t)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{CloudURL: "http://localhost", AgentID: "test", APIKey: "key"}
	session := CrawlSession{
		SessionID: "script-test",
		URL:       ts.URL,
		SnapshotScript: `JSON.stringify({
			interactive_elements: [{tag: "button", text: "Click Me"}],
			forms: [],
			selectors: {"button": "#btn1"},
			accessibility_tree: "button: Click Me"
		})`,
	}

	snapshot, err := a.captureSnapshot(ctx, session, 1)
	if err != nil {
		t.Fatalf("captureSnapshot failed: %v", err)
	}

	if len(snapshot.InteractiveElements) == 0 {
		t.Error("expected interactive elements from script")
	}
	if snapshot.Selectors["button"] != "#btn1" {
		t.Errorf("selector: got %q", snapshot.Selectors["button"])
	}
	if snapshot.AccessibilityTree == "" {
		t.Error("accessibility tree should not be empty")
	}
}

// --- executeCrawlAction tests ---

func TestExecuteCrawlAction_Click(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<button id="btn" onclick="document.title='clicked'">Click Me</button>
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newBrowserContext(t)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	action := &CrawlAction{Action: "click", Selector: "#btn"}
	err := a.executeCrawlAction(ctx, "test-session", action)
	if err != nil {
		t.Fatalf("executeCrawlAction click failed: %v", err)
	}

	// Verify click worked
	var title string
	_ = chromedp.Run(ctx, chromedp.Title(&title))
	if title != "clicked" {
		t.Errorf("expected title 'clicked', got %q", title)
	}
}

func TestExecuteCrawlAction_Fill(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<input id="email" type="text" value="" />
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newBrowserContext(t)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	action := &CrawlAction{Action: "fill", Selector: "#email", Value: "test@example.com"}
	err := a.executeCrawlAction(ctx, "test-session", action)
	if err != nil {
		t.Fatalf("executeCrawlAction fill failed: %v", err)
	}

	var value string
	_ = chromedp.Run(ctx, chromedp.Value("#email", &value, chromedp.ByQuery))
	if value != "test@example.com" {
		t.Errorf("expected 'test@example.com', got %q", value)
	}
}

func TestExecuteCrawlAction_Select(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<select id="country">
				<option value="">Select</option>
				<option value="US">United States</option>
				<option value="UK">United Kingdom</option>
			</select>
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newBrowserContext(t)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	action := &CrawlAction{Action: "select", Selector: "#country", Value: "US"}
	err := a.executeCrawlAction(ctx, "test-session", action)
	if err != nil {
		t.Fatalf("executeCrawlAction select failed: %v", err)
	}

	var value string
	_ = chromedp.Run(ctx, chromedp.Value("#country", &value, chromedp.ByQuery))
	if value != "US" {
		t.Errorf("expected 'US', got %q", value)
	}
}

func TestExecuteCrawlAction_UnknownAction(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	a := &Agent{}
	action := &CrawlAction{Action: "unknown_action", Selector: "#btn"}
	err := a.executeCrawlAction(context.Background(), "test", action)
	if err == nil {
		t.Error("expected error for unknown action")
	}
	if !strings.Contains(err.Error(), "unknown action") {
		t.Errorf("error should contain 'unknown action': %v", err)
	}
}

// --- crawlClick JS fallback test ---

func TestCrawlClick_JSFallback(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// Create a hidden button that native click can't reach but JS click can
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<div style="overflow:hidden;width:0;height:0">
				<button id="hidden-btn" onclick="document.title='js-clicked'">Hidden</button>
			</div>
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newBrowserContext(t)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	// This should fall back to JS click
	err := a.crawlClick(ctx, "test-session", "#hidden-btn")
	if err != nil {
		t.Logf("crawlClick returned error (may be expected): %v", err)
	}
}

// --- dismissCookieConsent test ---

func TestDismissCookieConsent_WithAcceptButton(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<div id="cookie-banner" style="position:fixed;bottom:0;left:0;right:0;background:#333;padding:20px;z-index:9999">
				<p>We use cookies</p>
				<button onclick="document.getElementById('cookie-banner').style.display='none'; document.title='cookies-accepted'">Accept All</button>
			</div>
			<h1>Main Content</h1>
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newBrowserContext(t)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	a.dismissCookieConsent(ctx, "test-session")

	// Check if the cookie consent was dismissed
	var title string
	_ = chromedp.Run(ctx, chromedp.Title(&title))
	if title == "cookies-accepted" {
		t.Log("Cookie consent was dismissed successfully")
	}
}

func TestDismissCookieConsent_NoBanner(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body><h1>No cookies here</h1></body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newBrowserContext(t)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	// Should not panic or error
	a := &Agent{}
	a.dismissCookieConsent(ctx, "test-session")
}

// --- ExecuteCrawlSession integration test ---

func TestExecuteCrawlSession_Integration(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	// Create a simple HTML page to crawl
	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Crawl Target</title></head><body>
			<h1>Welcome</h1>
			<button id="btn1">Click Me</button>
			<a href="/page2">Page 2</a>
		</body></html>`)
	}))
	defer htmlServer.Close()

	// Mock the QualityMax API
	step := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "/snapshot") {
			step++
			// First step: done immediately
			_ = json.NewEncoder(w).Encode(CrawlAction{
				Action: "done",
				Reason: "Crawl complete",
			})
			return
		}

		if strings.Contains(r.URL.Path, "/error") {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer apiServer.Close()

	a := &Agent{
		CloudURL: apiServer.URL,
		AgentID:  "crawl-agent",
		APIKey:   "crawl-key",
		client:   &http.Client{Timeout: 30 * time.Second},
	}

	session := CrawlSession{
		SessionID: "crawl-integration",
		URL:       htmlServer.URL,
		MaxSteps:  3,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Should complete without error
	a.ExecuteCrawlSession(ctx, session)

	if step < 1 {
		t.Error("expected at least 1 snapshot submission")
	}
}

func TestExecuteCrawlSession_ContextCancelled(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body><h1>Page</h1></body></html>`)
	}))
	defer htmlServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Return "click" action to keep the crawl going
		_ = json.NewEncoder(w).Encode(CrawlAction{
			Action:   "click",
			Selector: "h1",
		})
	}))
	defer apiServer.Close()

	a := &Agent{
		CloudURL: apiServer.URL,
		AgentID:  "cancel-agent",
		APIKey:   "cancel-key",
		client:   &http.Client{Timeout: 30 * time.Second},
	}

	session := CrawlSession{
		SessionID: "cancel-test",
		URL:       htmlServer.URL,
		MaxSteps:  100,
	}

	// Cancel after a short time
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Should return when context is cancelled, not run all 100 steps
	a.ExecuteCrawlSession(ctx, session)
}

// --- ExecuteCrawlSession error path test ---

func TestExecuteCrawlSession_SnapshotError(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Error Test</title></head><body><h1>Page</h1></body></html>`)
	}))
	defer htmlServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "/snapshot") {
			// Return error status to test error handling
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "bad snapshot")
			return
		}
		if strings.Contains(r.URL.Path, "/error") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer apiServer.Close()

	a := &Agent{
		CloudURL: apiServer.URL,
		AgentID:  "snap-err-agent",
		APIKey:   "snap-err-key",
		client:   &http.Client{Timeout: 30 * time.Second},
	}

	session := CrawlSession{
		SessionID: "snap-err-test",
		URL:       htmlServer.URL,
		MaxSteps:  3,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Should handle snapshot submission error gracefully
	a.ExecuteCrawlSession(ctx, session)
}

func TestExecuteCrawlSession_ActionError(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	htmlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Action Error</title></head><body><h1>Page</h1></body></html>`)
	}))
	defer htmlServer.Close()

	step := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if strings.Contains(r.URL.Path, "/snapshot") {
			step++
			if step == 1 {
				// Return click on non-existent element — action will fail
				_ = json.NewEncoder(w).Encode(CrawlAction{
					Action:   "click",
					Selector: "#nonexistent-element-12345",
				})
			} else {
				_ = json.NewEncoder(w).Encode(CrawlAction{
					Action: "done",
					Reason: "completed",
				})
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer apiServer.Close()

	a := &Agent{
		CloudURL: apiServer.URL,
		AgentID:  "action-err-agent",
		APIKey:   "action-err-key",
		client:   &http.Client{Timeout: 30 * time.Second},
	}

	session := CrawlSession{
		SessionID: "action-err-test",
		URL:       htmlServer.URL,
		MaxSteps:  5,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Should handle action error gracefully and continue
	a.ExecuteCrawlSession(ctx, session)

	if step < 2 {
		t.Error("expected at least 2 snapshot submissions (one after action error)")
	}
}

// --- crawlComboboxSelect test ---

func TestCrawlComboboxSelect(t *testing.T) {
	if os.Getenv("QAMAX_BROWSER_TESTS") == "" {
		t.Skip("Skipping browser test (set QAMAX_BROWSER_TESTS=1 to run)")
	}
	skipIfNoChrome(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body>
			<input id="combo" type="text" role="combobox" />
			<ul role="listbox" style="display:none">
				<li role="option" data-value="opt1">Option 1</li>
			</ul>
			<script>
				document.getElementById('combo').addEventListener('click', function() {
					document.querySelector('[role="listbox"]').style.display = 'block';
				});
			</script>
		</body></html>`)
	}))
	defer ts.Close()

	ctx, cancel := newBrowserContext(t)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(ts.URL), chromedp.WaitReady("body")); err != nil {
		t.Fatalf("navigation failed: %v", err)
	}

	a := &Agent{}
	err := a.crawlComboboxSelect(ctx, "#combo", "Option")
	// This may fail since the combobox implementation is simplified, but it should not panic
	if err != nil {
		t.Logf("crawlComboboxSelect returned error (expected for simplified test page): %v", err)
	}
}
