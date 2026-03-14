package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// --- Crawl data structures ---

// CrawlSession represents a pending crawl session from the server.
type CrawlSession struct {
	SessionID      string `json:"session_id"`
	URL            string `json:"url"`
	Instructions   string `json:"instructions"`
	MaxSteps       int    `json:"max_steps"`
	SnapshotScript string `json:"snapshot_script"`
}

// CrawlSnapshot is sent to the server after each crawl step.
type CrawlSnapshot struct {
	SessionID           string           `json:"session_id"`
	StepNum             int              `json:"step_num"`
	URL                 string           `json:"url"`
	Title               string           `json:"title"`
	InteractiveElements []map[string]any `json:"interactive_elements"`
	Forms               []map[string]any `json:"forms"`
	Selectors           map[string]string `json:"selectors"`
	ScreenshotBase64    string           `json:"screenshot_base64"`
	AccessibilityTree   string           `json:"accessibility_tree"`
}

// CrawlAction is the server's response telling the agent what to do next.
type CrawlAction struct {
	Action   string `json:"action"`
	Selector string `json:"selector"`
	Value    string `json:"value"`
	Reason   string `json:"reason"`
	StepNum  int    `json:"step_num"`
}

// --- HTTP helpers with retry ---

// doJSONWithRetry wraps doJSON with retry logic for 5xx errors.
func (a *Agent) doJSONWithRetry(method, url string, body interface{}, headers map[string]string, timeout time.Duration) (*http.Response, []byte, error) {
	// Use a dedicated client with the specified timeout
	client := &http.Client{Timeout: timeout}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		var reqBody io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				return nil, nil, fmt.Errorf("marshal request: %w", err)
			}
			reqBody = bytes.NewReader(data)
		}

		req, err := http.NewRequest(method, url, reqBody)
		if err != nil {
			return nil, nil, fmt.Errorf("create request: %w", err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("do request: %w", err)
			continue
		}

		const maxResponseBody = 50 * 1024 * 1024
		respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error: %d - %s", resp.StatusCode, string(respBody))
			log.Printf("WARN: Crawl HTTP %d on attempt %d, retrying...", resp.StatusCode, attempt+1)
			continue
		}

		return resp, respBody, nil
	}

	return nil, nil, fmt.Errorf("all retries exhausted: %w", lastErr)
}

// --- Polling ---

// PollCrawlSessions checks the server for pending crawl sessions.
func (a *Agent) PollCrawlSessions() (*CrawlSession, error) {
	if a.AgentID == "" || a.APIKey == "" {
		return nil, nil
	}

	url := fmt.Sprintf("%s/api/agent/%s/crawl/pending", a.CloudURL, a.AgentID)
	resp, body, err := a.doJSONWithRetry("GET", url, nil, a.authHeaders(), 10*time.Second)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("poll crawl sessions failed: %d - %s", resp.StatusCode, string(body))
	}

	var wrapper struct {
		Session *CrawlSession `json:"session"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("parse crawl session: %w", err)
	}

	if wrapper.Session == nil || wrapper.Session.SessionID == "" {
		return nil, nil
	}

	return wrapper.Session, nil
}

// --- Crawl execution ---

// ExecuteCrawlSession runs a discovery crawl using chromedp.
func (a *Agent) ExecuteCrawlSession(ctx context.Context, session CrawlSession) {
	log.Printf("CRAWL [%s] Starting crawl session: url=%s, max_steps=%d", session.SessionID, session.URL, session.MaxSteps)

	// Overall session timeout: 10 minutes
	sessionCtx, sessionCancel := context.WithTimeout(ctx, 10*time.Minute)
	defer sessionCancel()

	// Determine headless mode
	headed := strings.EqualFold(os.Getenv("QAMAX_CRAWL_HEADED"), "true")

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", !headed),
		chromedp.Flag("disable-gpu", true),
		chromedp.WindowSize(1280, 720),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(sessionCtx, opts...)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(func(s string, args ...interface{}) {
			log.Printf("CRAWL [%s] chromedp: "+s, append([]interface{}{session.SessionID}, args...)...)
		}),
	)
	defer browserCancel()

	// Set viewport
	if err := chromedp.Run(browserCtx,
		emulation.SetDeviceMetricsOverride(1280, 720, 1.0, false),
	); err != nil {
		log.Printf("CRAWL [%s] ERROR setting viewport: %v", session.SessionID, err)
		a.submitCrawlError(session.SessionID, fmt.Sprintf("failed to set viewport: %v", err))
		return
	}

	// Navigate to the target URL
	log.Printf("CRAWL [%s] Navigating to %s", session.SessionID, session.URL)
	if err := chromedp.Run(browserCtx,
		chromedp.Navigate(session.URL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(500*time.Millisecond),
	); err != nil {
		log.Printf("CRAWL [%s] ERROR navigating: %v", session.SessionID, err)
		a.submitCrawlError(session.SessionID, fmt.Sprintf("failed to navigate to %s: %v", session.URL, err))
		return
	}

	// Dismiss cookie consent overlays
	a.dismissCookieConsent(browserCtx, session.SessionID)

	// Main crawl loop
	for step := 1; step <= session.MaxSteps; step++ {
		select {
		case <-sessionCtx.Done():
			log.Printf("CRAWL [%s] Session context cancelled at step %d", session.SessionID, step)
			return
		default:
		}

		log.Printf("CRAWL [%s] Step %d/%d", session.SessionID, step, session.MaxSteps)

		// Capture snapshot
		snapshot, err := a.captureSnapshot(browserCtx, session, step)
		if err != nil {
			log.Printf("CRAWL [%s] ERROR capturing snapshot at step %d: %v", session.SessionID, step, err)
			a.submitCrawlError(session.SessionID, fmt.Sprintf("snapshot capture failed at step %d: %v", step, err))
			return
		}

		// Send snapshot to server and get next action
		action, err := a.submitSnapshot(session.SessionID, snapshot)
		if err != nil {
			log.Printf("CRAWL [%s] ERROR submitting snapshot at step %d: %v", session.SessionID, step, err)
			a.submitCrawlError(session.SessionID, fmt.Sprintf("snapshot submission failed at step %d: %v", step, err))
			return
		}

		log.Printf("CRAWL [%s] Step %d action: %s selector=%q value=%q reason=%q",
			session.SessionID, step, action.Action, action.Selector, action.Value, action.Reason)

		// Check if done
		if action.Action == "done" {
			log.Printf("CRAWL [%s] Crawl completed at step %d: %s", session.SessionID, step, action.Reason)
			return
		}

		// Execute the action
		if err := a.executeCrawlAction(browserCtx, session.SessionID, action); err != nil {
			log.Printf("CRAWL [%s] ERROR executing action at step %d: %v", session.SessionID, step, err)
			// Don't abort on action failure — let the server decide on the next snapshot
		}

		// Wait for page to settle after action
		_ = chromedp.Run(browserCtx, chromedp.Sleep(1*time.Second))
	}

	log.Printf("CRAWL [%s] Reached max steps (%d)", session.SessionID, session.MaxSteps)
}

// captureSnapshot takes a screenshot and evaluates the snapshot script.
func (a *Agent) captureSnapshot(ctx context.Context, session CrawlSession, stepNum int) (*CrawlSnapshot, error) {
	actionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Get current URL and title
	var currentURL, title string
	if err := chromedp.Run(actionCtx,
		chromedp.Location(&currentURL),
		chromedp.Title(&title),
	); err != nil {
		return nil, fmt.Errorf("get page info: %w", err)
	}

	// Take screenshot
	var screenshotBuf []byte
	if err := chromedp.Run(actionCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		screenshotBuf, err = page.CaptureScreenshot().
			WithFormat(page.CaptureScreenshotFormatPng).
			WithClip(&page.Viewport{
				X: 0, Y: 0, Width: 1280, Height: 720, Scale: 1,
			}).
			Do(ctx)
		return err
	})); err != nil {
		return nil, fmt.Errorf("capture screenshot: %w", err)
	}

	snapshot := &CrawlSnapshot{
		SessionID:        session.SessionID,
		StepNum:          stepNum,
		URL:              currentURL,
		Title:            title,
		ScreenshotBase64: base64.StdEncoding.EncodeToString(screenshotBuf),
	}

	// Evaluate snapshot script if provided
	if session.SnapshotScript != "" {
		scriptCtx, scriptCancel := context.WithTimeout(ctx, 5*time.Second)
		defer scriptCancel()

		var result string
		if err := chromedp.Run(scriptCtx,
			chromedp.Evaluate(session.SnapshotScript, &result),
		); err != nil {
			log.Printf("CRAWL [%s] WARN: snapshot script evaluation failed: %v", session.SessionID, err)
		} else {
			// Parse the JSON result from the script
			var scriptData struct {
				InteractiveElements []map[string]any  `json:"interactive_elements"`
				Forms               []map[string]any  `json:"forms"`
				Selectors           map[string]string `json:"selectors"`
				AccessibilityTree   string            `json:"accessibility_tree"`
			}
			if err := json.Unmarshal([]byte(result), &scriptData); err != nil {
				log.Printf("CRAWL [%s] WARN: failed to parse snapshot script result: %v", session.SessionID, err)
			} else {
				snapshot.InteractiveElements = scriptData.InteractiveElements
				snapshot.Forms = scriptData.Forms
				snapshot.Selectors = scriptData.Selectors
				snapshot.AccessibilityTree = scriptData.AccessibilityTree
			}
		}
	}

	return snapshot, nil
}

// submitSnapshot sends a snapshot to the server and returns the next action.
func (a *Agent) submitSnapshot(sessionID string, snapshot *CrawlSnapshot) (*CrawlAction, error) {
	url := fmt.Sprintf("%s/api/agent/%s/crawl/%s/snapshot", a.CloudURL, a.AgentID, sessionID)

	resp, body, err := a.doJSONWithRetry("POST", url, snapshot, a.authHeaders(), 60*time.Second)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("submit snapshot failed: %d - %s", resp.StatusCode, string(body))
	}

	var action CrawlAction
	if err := json.Unmarshal(body, &action); err != nil {
		return nil, fmt.Errorf("parse crawl action: %w", err)
	}

	return &action, nil
}

// submitCrawlError reports a crawl error to the server.
func (a *Agent) submitCrawlError(sessionID, errorMsg string) {
	url := fmt.Sprintf("%s/api/agent/%s/crawl/%s/error", a.CloudURL, a.AgentID, sessionID)
	payload := map[string]string{"error": errorMsg}

	resp, _, err := a.doJSONWithRetry("POST", url, payload, a.authHeaders(), 10*time.Second)
	if err != nil {
		log.Printf("CRAWL [%s] ERROR reporting crawl error: %v", sessionID, err)
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("CRAWL [%s] ERROR crawl error report failed: %d", sessionID, resp.StatusCode)
	}
}

// --- Action execution ---

// executeCrawlAction performs the browser action specified by the server.
func (a *Agent) executeCrawlAction(ctx context.Context, sessionID string, action *CrawlAction) error {
	actionCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	switch action.Action {
	case "click":
		return a.crawlClick(actionCtx, sessionID, action.Selector)
	case "fill":
		return a.crawlFill(actionCtx, action.Selector, action.Value)
	case "select":
		return a.crawlSelect(actionCtx, action.Selector, action.Value)
	case "combobox_select":
		return a.crawlComboboxSelect(ctx, action.Selector, action.Value)
	default:
		return fmt.Errorf("unknown action: %s", action.Action)
	}
}

// crawlClick clicks an element, falling back to JS click on failure.
func (a *Agent) crawlClick(ctx context.Context, sessionID, selector string) error {
	err := chromedp.Run(ctx, chromedp.Click(selector, chromedp.ByQuery))
	if err != nil {
		log.Printf("CRAWL [%s] WARN: native click failed on %q, trying JS click: %v", sessionID, selector, err)
		// Fallback: JS click
		jsClick := fmt.Sprintf(`document.querySelector(%q)?.click()`, selector)
		var result interface{}
		return chromedp.Run(ctx, chromedp.Evaluate(jsClick, &result))
	}
	return nil
}

// crawlFill clears a field and types the given value.
func (a *Agent) crawlFill(ctx context.Context, selector, value string) error {
	return chromedp.Run(ctx,
		chromedp.Clear(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, value, chromedp.ByQuery),
	)
}

// crawlSelect sets the value of a native <select> element.
func (a *Agent) crawlSelect(ctx context.Context, selector, value string) error {
	return chromedp.Run(ctx,
		chromedp.SetValue(selector, value, chromedp.ByQuery),
	)
}

// crawlComboboxSelect handles custom combobox/dropdown components.
func (a *Agent) crawlComboboxSelect(ctx context.Context, selector, value string) error {
	// Click the combobox trigger
	clickCtx, clickCancel := context.WithTimeout(ctx, 5*time.Second)
	defer clickCancel()
	if err := chromedp.Run(clickCtx, chromedp.Click(selector, chromedp.ByQuery)); err != nil {
		return fmt.Errorf("click combobox trigger: %w", err)
	}

	// Wait for dropdown to appear
	_ = chromedp.Run(ctx, chromedp.Sleep(300*time.Millisecond))

	// Type the value to filter options
	typeCtx, typeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer typeCancel()
	if err := chromedp.Run(typeCtx, chromedp.SendKeys(selector, value, chromedp.ByQuery)); err != nil {
		return fmt.Errorf("type combobox value: %w", err)
	}

	// Wait for options to filter
	_ = chromedp.Run(ctx, chromedp.Sleep(300*time.Millisecond))

	// Click the first visible option
	optionCtx, optionCancel := context.WithTimeout(ctx, 5*time.Second)
	defer optionCancel()

	// Try common option selectors
	optionSelectors := []string{
		`[role="option"]:not([aria-hidden="true"])`,
		`[role="listbox"] [role="option"]`,
		`.option:not(.hidden)`,
		`li[data-value]`,
	}

	for _, optSel := range optionSelectors {
		if err := chromedp.Run(optionCtx, chromedp.Click(optSel, chromedp.ByQuery)); err == nil {
			return nil
		}
	}

	return fmt.Errorf("could not find a visible option to click after typing %q", value)
}

// --- Cookie consent dismissal ---

// dismissCookieConsent attempts to dismiss common cookie consent overlays.
func (a *Agent) dismissCookieConsent(ctx context.Context, sessionID string) {
	// Try each button text with exact matching to avoid false positives
	buttonTexts := []string{
		"Accept All",
		"Accept all",
		"Accept All Cookies",
		"Accept all cookies",
		"Accept Cookies",
		"Accept cookies",
		"I Accept",
		"I Agree",
		"Agree",
		"Allow All",
		"Allow all",
		"Got it",
		"OK",
	}

	for _, text := range buttonTexts {
		// Use XPath to find buttons with exact text content
		xpath := fmt.Sprintf(
			`//button[normalize-space(.)=%q] | //a[normalize-space(.)=%q] | //*[@role="button"][normalize-space(.)=%q]`,
			text, text, text,
		)

		clickCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		err := chromedp.Run(clickCtx, chromedp.Click(xpath, chromedp.BySearch))
		cancel()

		if err == nil {
			log.Printf("CRAWL [%s] Dismissed cookie consent with %q", sessionID, text)
			// Wait for overlay to disappear
			_ = chromedp.Run(ctx, chromedp.Sleep(500*time.Millisecond))
			return
		}
	}

	log.Printf("CRAWL [%s] No cookie consent overlay detected", sessionID)
}
