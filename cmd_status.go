package main

import (
	"flag"
	"fmt"
	"os"
)

func cmdStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	_ = fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	path, _ := ConfigPath()
	fmt.Printf("Config: %s\n", path)
	fmt.Println()

	// Auth status
	if cfg.Token != "" {
		fmt.Println("Auth:   logged in (OAuth token present)")
	} else {
		fmt.Println("Auth:   not logged in")
	}

	// API URL
	if cfg.APIURL != "" {
		fmt.Printf("API:    %s\n", cfg.APIURL)
	} else {
		fmt.Println("API:    not configured")
	}

	// Agent status
	if cfg.AgentID != "" {
		fmt.Printf("Agent:  registered (ID: %s)\n", cfg.AgentID)
	} else {
		fmt.Println("Agent:  not registered")
	}

	if cfg.APIKey != "" {
		if len(cfg.APIKey) >= 8 {
			fmt.Printf("Key:    %s...%s\n", cfg.APIKey[:4], cfg.APIKey[len(cfg.APIKey)-4:])
		} else {
			fmt.Println("Key:    ****")
		}
	}
}
