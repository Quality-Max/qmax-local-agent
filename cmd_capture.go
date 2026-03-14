package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

func cmdCapture(args []string) {
	fs := flag.NewFlagSet("capture", flag.ExitOnError)
	projectID := fs.String("project-id", "", "Project ID (required)")
	name := fs.String("name", "", "Name for the auth data field (required)")
	output := fs.String("output", "", "Optional file path to write storage state JSON")
	urlFlag := fs.String("url", "", "Target URL to capture cookies from")
	_ = fs.Parse(args)

	// URL can be positional (last arg) or via --url flag
	targetURL := *urlFlag
	if targetURL == "" {
		targetURL = fs.Arg(0)
	}
	if targetURL == "" {
		fmt.Fprintln(os.Stderr, "Error: URL argument is required")
		fmt.Fprintln(os.Stderr, "Usage: qmax capture --project-id ID --name NAME [--output FILE] <url>")
		fmt.Fprintln(os.Stderr, "   or: qmax capture --url URL --project-id ID --name NAME")
		os.Exit(1)
	}
	if *projectID == "" {
		fmt.Fprintln(os.Stderr, "Error: --project-id is required")
		os.Exit(1)
	}
	if *name == "" {
		fmt.Fprintln(os.Stderr, "Error: --name is required")
		os.Exit(1)
	}

	cfg, err := LoadConfig()
	if err != nil || cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: not logged in. Run `qmax login` first.")
		os.Exit(1)
	}

	// Launch visible Chrome
	fmt.Println("Launching Chrome...")
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", false),
	)

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(func(string, ...interface{}) {}),
		chromedp.WithErrorf(func(string, ...interface{}) {}),
	)
	defer cancel()

	// Navigate to URL
	fmt.Printf("Navigating to %s\n", targetURL)
	if err := chromedp.Run(ctx, chromedp.Navigate(targetURL)); err != nil {
		fmt.Fprintf(os.Stderr, "Error navigating: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nA fresh Chrome window has opened (no existing sessions).")
	fmt.Println("Complete the full login flow in the browser window.")
	fmt.Println("Once you see the authenticated page, come back here.")
	fmt.Print("\nPress ENTER when login is complete...")

	buf := make([]byte, 1)
	for {
		n, readErr := os.Stdin.Read(buf)
		if n > 0 && buf[0] == '\n' {
			break
		}
		if readErr != nil {
			// stdin is closed/piped — wait for browser to be closed instead
			fmt.Println("\n(stdin unavailable — close the Chrome window when done)")
			<-ctx.Done()
			break
		}
	}

	// Navigate back to target URL to ensure we're on the right domain
	fmt.Printf("\nNavigating back to %s...\n", targetURL)
	if err := chromedp.Run(ctx, chromedp.Navigate(targetURL)); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: could not navigate back: %v\n", err)
	}
	// Brief wait for cookies to settle after navigation
	_ = chromedp.Run(ctx, chromedp.Sleep(2*time.Second))

	// Capture ALL browser cookies (including HTTP-Only from all domains visited)
	fmt.Println("Capturing cookies...")
	var cookies []*network.Cookie
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		cookies, err = storage.GetCookies().Do(ctx)
		return err
	})); err != nil {
		fmt.Fprintf(os.Stderr, "Error capturing cookies: %v\n", err)
		os.Exit(1)
	}

	// Also capture localStorage from the current page
	fmt.Println("Capturing localStorage...")
	var localStorageJSON string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`JSON.stringify(
		Object.keys(localStorage).map(k => ({name: k, value: localStorage.getItem(k)}))
	)`, &localStorageJSON)); err != nil {
		fmt.Fprintf(os.Stderr, "WARN: could not capture localStorage: %v\n", err)
		localStorageJSON = "[]"
	}

	cancel()
	allocCancel()

	fmt.Printf("Captured %d cookies\n", len(cookies))

	// Parse localStorage entries
	var localStorageEntries []map[string]string
	_ = json.Unmarshal([]byte(localStorageJSON), &localStorageEntries)
	if len(localStorageEntries) > 0 {
		fmt.Printf("Captured %d localStorage entries\n", len(localStorageEntries))
	}

	// Convert to Playwright storage state format
	storageState := buildPlaywrightStorageState(cookies, targetURL, localStorageEntries)

	stateJSON, err := json.MarshalIndent(storageState, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling storage state: %v\n", err)
		os.Exit(1)
	}

	// Optionally write to file
	if *output != "" {
		if err := os.WriteFile(*output, stateJSON, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output file: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Storage state written to %s\n", *output)
	}

	// Upload to QualityMax
	fmt.Println("Uploading auth data to QualityMax...")
	if err := uploadAuthData(cfg, *projectID, *name, string(stateJSON)); err != nil {
		fmt.Fprintf(os.Stderr, "Error uploading: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Auth data uploaded successfully!")
}

type playwrightCookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Expires  float64 `json:"expires"`
	HTTPOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	SameSite string  `json:"sameSite"`
}

type playwrightStorageState struct {
	Cookies []playwrightCookie   `json:"cookies"`
	Origins []map[string]interface{} `json:"origins"`
}

func buildPlaywrightStorageState(cookies []*network.Cookie, originURL string, localStorage []map[string]string) playwrightStorageState {
	state := playwrightStorageState{
		Cookies: make([]playwrightCookie, 0, len(cookies)),
		Origins: []map[string]interface{}{},
	}

	for _, c := range cookies {
		sameSite := "None"
		switch c.SameSite {
		case network.CookieSameSiteLax:
			sameSite = "Lax"
		case network.CookieSameSiteStrict:
			sameSite = "Strict"
		}

		state.Cookies = append(state.Cookies, playwrightCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Expires:  c.Expires,
			HTTPOnly: c.HTTPOnly,
			Secure:   c.Secure,
			SameSite: sameSite,
		})
	}

	// Include localStorage entries as Playwright origin storage
	if len(localStorage) > 0 {
		// Extract origin from URL (scheme + host)
		origin := originURL
		if idx := strings.Index(originURL, "://"); idx != -1 {
			rest := originURL[idx+3:]
			if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
				origin = originURL[:idx+3+slashIdx]
			}
		}
		state.Origins = append(state.Origins, map[string]interface{}{
			"origin":       origin,
			"localStorage": localStorage,
		})
	}

	return state
}

func uploadAuthData(cfg *Config, projectID, fieldName, storageStateJSON string) error {
	apiURL := cfg.GetAPIBaseURL()
	client := &http.Client{Timeout: 30 * time.Second}
	authHeader := fmt.Sprintf("Bearer %s", cfg.Token)

	// Step 1: Find or create "Authentication" category
	catID, err := findOrCreateAuthCategory(client, apiURL, projectID, authHeader)
	if err != nil {
		return fmt.Errorf("category setup: %w", err)
	}

	// Step 2: Create secret field with the storage state
	fieldURL := fmt.Sprintf("%s/api/projects/%s/user-data/categories/%s/fields", apiURL, projectID, catID)
	fieldPayload := map[string]interface{}{
		"key":       fieldName,
		"value":     storageStateJSON,
		"is_secret": true,
	}

	body, err := json.Marshal(fieldPayload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", fieldURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("create field request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		return fmt.Errorf("create field failed: %d - %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func findOrCreateAuthCategory(client *http.Client, apiURL, projectID, authHeader string) (string, error) {
	// GET existing categories
	listURL := fmt.Sprintf("%s/api/projects/%s/user-data/all", apiURL, projectID)
	req, err := http.NewRequest("GET", listURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("list categories: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("list categories failed: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return "", err
	}

	var response struct {
		Categories []struct {
			ID   json.Number `json:"id"`
			Name string      `json:"name"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("parse categories: %w", err)
	}

	// Look for existing "Authentication" category
	for _, cat := range response.Categories {
		if strings.EqualFold(cat.Name, "Authentication") {
			return cat.ID.String(), nil
		}
	}

	// Create it
	createURL := fmt.Sprintf("%s/api/projects/%s/user-data/categories", apiURL, projectID)
	createPayload, _ := json.Marshal(map[string]string{"name": "Authentication"})
	createReq, err := http.NewRequest("POST", createURL, bytes.NewReader(createPayload))
	if err != nil {
		return "", err
	}
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("Authorization", authHeader)

	createResp, err := client.Do(createReq)
	if err != nil {
		return "", fmt.Errorf("create category: %w", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(createResp.Body)
		return "", fmt.Errorf("create category failed: %d - %s", createResp.StatusCode, string(respBody))
	}

	createBody, err := io.ReadAll(io.LimitReader(createResp.Body, 1024*1024))
	if err != nil {
		return "", err
	}

	var created struct {
		Category struct {
			ID json.Number `json:"id"`
		} `json:"category"`
	}
	if err := json.Unmarshal(createBody, &created); err != nil {
		return "", fmt.Errorf("parse created category: %w", err)
	}

	return created.Category.ID.String(), nil
}
