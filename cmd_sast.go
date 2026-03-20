package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// SastVerifyResult represents the server verification response.
type SastVerifyResult struct {
	OverallStatus  string       `json:"overall_status"`
	Tools          []ToolStatus `json:"tools"`
	TestScanPassed bool         `json:"test_scan_passed"`
	TestScanFinds  int          `json:"test_scan_findings"`
	Errors         []string     `json:"errors"`
}

type ToolStatus struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Version   string `json:"version"`
	Path      string `json:"path"`
	Error     string `json:"error"`
}

func cmdSast(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: qmax sast <subcommand>")
		fmt.Println()
		fmt.Println("Subcommands:")
		fmt.Println("  verify     Verify SAST installation (tools, webhook, scan)")
		fmt.Println("  install    Install missing SAST tools")
		fmt.Println("  scan       Run a local SAST scan and report results")
		fmt.Println("  setup      Generate CI/CD pipeline config and push to repo")
		os.Exit(1)
	}

	sub := args[0]
	switch sub {
	case "verify":
		cmdSastVerify(args[1:])
	case "install":
		cmdSastInstall(args[1:])
	case "scan":
		cmdSastScan(args[1:])
	case "setup":
		cmdSastSetup(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown sast subcommand: %s\n", sub)
		os.Exit(1)
	}
}

func cmdSastVerify(args []string) {
	cfg, err := LoadConfig()
	if err != nil || cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "Not logged in. Run: qmax login")
		os.Exit(1)
	}

	fmt.Println("=== QualityMax SAST Verification ===")
	fmt.Println()

	// 1. Check local tools
	fmt.Println("[1/4] Checking local SAST tools...")
	tools := checkLocalTools()
	allInstalled := true
	for _, t := range tools {
		status := "✓"
		if !t.Installed {
			status = "✗"
			allInstalled = false
		}
		fmt.Printf("  %s %s: %s\n", status, t.Name, t.Version)
		if t.Error != "" {
			fmt.Printf("    Error: %s\n", t.Error)
		}
	}
	fmt.Println()

	// 2. Verify server connectivity
	fmt.Println("[2/4] Verifying server connectivity...")
	serverOK := checkServerConnectivity(cfg)
	if serverOK {
		fmt.Println("  ✓ Server reachable")
	} else {
		fmt.Println("  ✗ Server unreachable")
	}
	fmt.Println()

	// 3. Run server-side verification
	fmt.Println("[3/4] Running server-side verification...")
	if serverOK {
		result := runServerVerification(cfg)
		if result != nil {
			fmt.Printf("  Server status: %s\n", result.OverallStatus)
			fmt.Printf("  Test scan: %v (%d findings)\n", result.TestScanPassed, result.TestScanFinds)
			if len(result.Errors) > 0 {
				for _, e := range result.Errors {
					fmt.Printf("  Error: %s\n", e)
				}
			}
		} else {
			fmt.Println("  ✗ Server verification failed")
		}
	} else {
		fmt.Println("  Skipped (server unreachable)")
	}
	fmt.Println()

	// 4. Summary
	fmt.Println("[4/4] Summary")
	if allInstalled && serverOK {
		fmt.Println("  ✓ SAST installation is complete and working!")
	} else {
		fmt.Println("  ⚠ Some issues found:")
		if !allInstalled {
			fmt.Println("    - Missing tools. Run: qmax sast install")
		}
		if !serverOK {
			fmt.Println("    - Server connectivity issue. Check your API key and network.")
		}
	}
}

func cmdSastInstall(args []string) {
	fmt.Println("=== Installing SAST Tools ===")
	fmt.Println()

	tools := checkLocalTools()
	for _, t := range tools {
		if t.Installed {
			fmt.Printf("✓ %s already installed (%s)\n", t.Name, t.Version)
			continue
		}

		fmt.Printf("Installing %s...\n", t.Name)
		switch t.Name {
		case "semgrep":
			installTool("pip", "install", "semgrep")
		case "bandit":
			installTool("pip", "install", "bandit")
		case "gitleaks":
			fmt.Println("  Install gitleaks from: https://github.com/gitleaks/gitleaks#installing")
			fmt.Println("  Homebrew: brew install gitleaks")
			fmt.Println("  Go: go install github.com/zricethezav/gitleaks/v8@latest")
		}
	}
}

func cmdSastScan(args []string) {
	cfg, err := LoadConfig()
	if err != nil || cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "Not logged in. Run: qmax login")
		os.Exit(1)
	}

	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	fmt.Printf("Running SAST scan on: %s\n", dir)

	// Run local scans
	tools := checkLocalTools()
	totalFindings := 0

	for _, t := range tools {
		if !t.Installed {
			continue
		}
		fmt.Printf("\nRunning %s...\n", t.Name)
		findings := runLocalScan(t.Name, dir)
		totalFindings += findings
		fmt.Printf("  %s: %d findings\n", t.Name, findings)
	}

	fmt.Printf("\nTotal findings: %d\n", totalFindings)

	// Report results to server
	if cfg.GetAPIBaseURL() != "" {
		fmt.Println("Reporting results to QualityMax...")
		reportScanResults(cfg, dir, totalFindings)
	}
}

func cmdSastSetup(args []string) {
	cfg, err := LoadConfig()
	if err != nil || cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "Not logged in. Run: qmax login")
		os.Exit(1)
	}

	platform := "github_actions"
	if len(args) > 0 {
		platform = args[0]
	}

	fmt.Printf("Generating %s pipeline config...\n", platform)

	body, _ := json.Marshal(map[string]interface{}{
		"platform":        platform,
		"scan_on_pr":      true,
		"scan_on_push":    false,
		"fail_on_findings": true,
		"severity_threshold": "medium",
	})

	req, _ := http.NewRequest("POST", cfg.GetAPIBaseURL()+"/api/sast/pipeline-config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate config: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var result struct {
		Filename     string `json:"filename"`
		Content      string `json:"content"`
		Instructions string `json:"instructions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nFile: %s\n", result.Filename)
	fmt.Println("---")
	fmt.Println(result.Content)
	fmt.Println("---")
	fmt.Printf("\nInstructions: %s\n", result.Instructions)
}

// =====================================================
// HELPERS
// =====================================================

func checkLocalTools() []ToolStatus {
	tools := []ToolStatus{
		checkTool("semgrep", "semgrep", "--version"),
		checkTool("bandit", "bandit", "--version"),
		checkTool("gitleaks", "gitleaks", "version"),
	}
	return tools
}

func checkTool(name, binary string, versionArgs ...string) ToolStatus {
	path, err := exec.LookPath(binary)
	if err != nil {
		return ToolStatus{Name: name, Installed: false, Error: "not found on PATH"}
	}

	args := append([]string{}, versionArgs...)
	cmd := exec.Command(binary, args...)
	out, err := cmd.CombinedOutput()
	version := strings.TrimSpace(string(out))
	if err != nil {
		return ToolStatus{Name: name, Installed: true, Path: path, Error: err.Error()}
	}
	// Take first line
	if idx := strings.Index(version, "\n"); idx > 0 {
		version = version[:idx]
	}
	return ToolStatus{Name: name, Installed: true, Version: version, Path: path}
}

func checkServerConnectivity(cfg *Config) bool {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", cfg.GetAPIBaseURL()+"/api/sast/verify/tools", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func runServerVerification(cfg *Config) *SastVerifyResult {
	client := &http.Client{Timeout: 60 * time.Second}
	req, _ := http.NewRequest("POST", cfg.GetAPIBaseURL()+"/api/sast/verify", nil)
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result SastVerifyResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}
	return &result
}

func installTool(cmd string, args ...string) {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "  Install failed: %v\n", err)
	} else {
		fmt.Println("  ✓ Installed successfully")
	}
}

func runLocalScan(toolName, dir string) int {
	switch toolName {
	case "semgrep":
		return runAndCountFindings("semgrep", "--json", "--metrics=off", "-q", "--config", "auto", dir)
	case "bandit":
		return runAndCountFindings("bandit", "-r", dir, "-f", "json", "-q")
	case "gitleaks":
		return runAndCountFindings("gitleaks", "detect", "--source", dir, "--no-git", "--report-format", "json", "--exit-code", "0")
	}
	return 0
}

func runAndCountFindings(binary string, args ...string) int {
	cmd := exec.Command(binary, args...)
	out, _ := cmd.CombinedOutput()

	// Try to parse JSON and count results
	var data map[string]interface{}
	if json.Unmarshal(out, &data) == nil {
		if results, ok := data["results"]; ok {
			if arr, ok := results.([]interface{}); ok {
				return len(arr)
			}
		}
	}

	// Try as array (gitleaks)
	var arr []interface{}
	if json.Unmarshal(out, &arr) == nil {
		return len(arr)
	}

	return 0
}

func reportScanResults(cfg *Config, dir string, findings int) {
	body, _ := json.Marshal(map[string]interface{}{
		"directory": dir,
		"findings":  findings,
		"agent_id":  cfg.AgentID,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})

	req, _ := http.NewRequest("POST", cfg.GetAPIBaseURL()+"/api/sast/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to report: %v\n", err)
		return
	}
	defer resp.Body.Close()
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing server response: %v\n", err)
		return
	}
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("Server response: %s\n", string(resultJSON))
}
