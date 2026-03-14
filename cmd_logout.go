package main

import (
	"flag"
	"fmt"
	"os"
)

func cmdLogout(args []string) {
	fs := flag.NewFlagSet("logout", flag.ExitOnError)
	_ = fs.Parse(args)

	path, err := ConfigPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Already logged out (no config file found).")
			return
		}
		fmt.Fprintf(os.Stderr, "Error removing config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Logged out. Config removed.")
}
