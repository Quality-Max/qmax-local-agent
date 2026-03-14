package main

import (
	"flag"
	"fmt"
	"os"
)

func cmdToken(args []string) {
	fs := flag.NewFlagSet("token", flag.ExitOnError)
	_ = fs.Parse(args)

	cfg, err := LoadConfig()
	if err != nil || cfg.Token == "" {
		fmt.Fprintln(os.Stderr, "Error: not logged in. Run `qmax login` first.")
		os.Exit(1)
	}

	fmt.Print(cfg.Token)
}
