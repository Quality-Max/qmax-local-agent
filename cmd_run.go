package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)

	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("WARN: could not load config: %v", err)
		cfg = &Config{}
	}

	// Defaults from config
	defaultURL := cfg.GetAPIBaseURL()
	defaultAgentID := cfg.AgentID
	defaultAPIKey := cfg.APIKey
	defaultSecret := cfg.RegistrationSecret

	cloudURL := fs.String("cloud-url", defaultURL, "QualityMax cloud URL (e.g., https://app.qualitymax.io)")
	apiKey := fs.String("api-key", defaultAPIKey, "Agent API key (optional, will be generated on first registration)")
	agentID := fs.String("agent-id", defaultAgentID, "Agent ID (optional, will be generated on first registration)")
	registrationSecret := fs.String("registration-secret", defaultSecret, "Registration secret (must match AGENT_REGISTRATION_SECRET on server)")
	pollInterval := fs.Int("poll-interval", 5, "Polling interval in seconds")
	heartbeatInterval := fs.Int("heartbeat-interval", 60, "Heartbeat interval in seconds")
	fs.Parse(args)

	if *cloudURL == "" {
		fmt.Fprintln(os.Stderr, "Error: --cloud-url is required (set via flag or `qamax-agent login`)")
		fs.Usage()
		os.Exit(1)
	}

	agent := NewAgent(
		*cloudURL,
		*apiKey,
		*agentID,
		*registrationSecret,
		time.Duration(*pollInterval)*time.Second,
		time.Duration(*heartbeatInterval)*time.Second,
	)

	// Save credentials back to config after successful registration
	agent.OnRegistered = func(newAgentID, newAPIKey string) {
		cfg.AgentID = newAgentID
		cfg.APIKey = newAPIKey
		if cfg.APIURL == "" {
			cfg.APIURL = *cloudURL
		}
		if cfg.RegistrationSecret == "" && *registrationSecret != "" {
			cfg.RegistrationSecret = *registrationSecret
		}
		if err := cfg.Save(); err != nil {
			log.Printf("WARN: could not save config: %v", err)
		} else {
			log.Printf("Credentials saved to config")
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Agent stopped by user")
		cancel()
	}()

	if err := agent.Run(ctx); err != nil {
		log.Fatalf("Agent error: %v", err)
	}
}
