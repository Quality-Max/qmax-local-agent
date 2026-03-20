package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func cmdProjects(args []string) {
	fs := flag.NewFlagSet("projects", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "Output raw JSON")
	_ = fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil || cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: not logged in. Run `qmax login` first.")
		os.Exit(1)
	}

	apiURL := cfg.GetAPIBaseURL()
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/projects", apiURL), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.Token))

	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		fmt.Fprintf(os.Stderr, "Error: %d - %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	const maxBody = 10 * 1024 * 1024 // 10MB
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
		os.Exit(1)
	}

	if *jsonOut {
		fmt.Println(string(body))
		return
	}

	var response struct {
		Projects []struct {
			ID   json.Number `json:"id"`
			Name string      `json:"name"`
			Slug string      `json:"slug"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing response: %v\n", err)
		os.Exit(1)
	}

	if len(response.Projects) == 0 {
		fmt.Println("No projects found.")
		return
	}

	fmt.Printf("%-8s  %-14s  %s\n", "ID", "Slug", "Name")
	fmt.Printf("%-8s  %-14s  %s\n", "--------", "--------------", "----")
	for _, p := range response.Projects {
		fmt.Printf("%-8s  %-14s  %s\n", p.ID.String(), p.Slug, p.Name)
	}
}
