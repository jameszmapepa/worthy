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
package main

import (
	"context"
	"fmt"
	"os"

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
// exit path stays a single place.
func run(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: repohealth <owner/repo | github.com/owner/repo | https://github.com/owner/repo>")
	}
	owner, repo, err := parseRepoArg(args[0])
	if err != nil {
		return err
	}
	client := github.NewClient()
	return tui.Run(context.Background(), client, owner, repo)
}
