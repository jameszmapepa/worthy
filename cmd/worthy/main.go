// Command worthy scores a public GitHub repository for maintenance health and contributor-friendliness.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jameszmapepa/worthy/internal/github"
	"github.com/jameszmapepa/worthy/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "worthy:", err)
		os.Exit(2)
	}
}

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
		return fmt.Errorf("usage: worthy [--ascii] <owner/repo | github.com/owner/repo | https://github.com/owner/repo>")
	}
	owner, repo, err := parseRepoArg(positional[0])
	if err != nil {
		return err
	}
	client := github.NewClient()
	return tui.Run(context.Background(), client, owner, repo, tui.WithASCIIIcons(ascii))
}

func asciiFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("WORTHY_ASCII"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
