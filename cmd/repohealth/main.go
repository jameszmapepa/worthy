// Command repohealth scores the health of a public GitHub repository and
// renders it as a terminal UI.
//
// Usage:
//
//	repohealth owner/repo
//	repohealth https://github.com/owner/repo
//	repohealth github.com/owner/repo
//
// A GITHUB_TOKEN environment variable, if present, lifts the API rate limit
// from 60 to 5,000 requests/hour. It is never required.
//
// Flags:
//
//	--ascii   render the language as a short ASCII tag (e.g. "TS") instead of a
//	          Nerd Font devicon glyph. Use this if your terminal font is not a
//	          Nerd Font (a program cannot detect that, so it must be told).
//	          REPO_HEALTH_ASCII=1 sets the same mode via the environment.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jameszmapepa/repo-health/internal/github"
	"github.com/jameszmapepa/repo-health/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "repohealth:", err)
		os.Exit(2)
	}
}

// run parses arguments and launches the TUI. It is separated from main so the
// exit path stays a single place. The --ascii flag (or REPO_HEALTH_ASCII env)
// selects the ASCII-tag language badge; everything else must be a single
// owner/repo positional.
func run(args []string) error {
	ascii := asciiFromEnv()
	positional := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "--ascii", "-a":
			ascii = true
		case "--no-ascii":
			ascii = false
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		return fmt.Errorf("usage: repohealth [--ascii] <owner/repo | github.com/owner/repo | https://github.com/owner/repo>")
	}
	owner, repo, err := parseRepoArg(positional[0])
	if err != nil {
		return err
	}
	client := github.NewClient()
	return tui.Run(context.Background(), client, owner, repo, tui.WithASCIIIcons(ascii))
}

// asciiFromEnv reports whether REPO_HEALTH_ASCII requests ASCII-tag mode.
func asciiFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("REPO_HEALTH_ASCII"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
