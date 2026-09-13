// Command worthy scores a public GitHub repository for maintenance health and contributor-friendliness.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

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
	noCache := envTruthy("WORTHY_NO_CACHE")
	positional := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "--ascii", "-a":
			ascii = true
		case "--no-ascii":
			ascii = false
		case "--no-cache":
			noCache = true
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) != 1 {
		return fmt.Errorf("usage: worthy [--ascii] [--no-cache] <owner/repo | github.com/owner/repo | https://github.com/owner/repo>")
	}
	owner, repo, err := parseRepoArg(positional[0])
	if err != nil {
		return err
	}
	client := github.NewClient(clientOptions(noCache)...)
	return tui.Run(context.Background(), client, owner, repo, tui.WithASCIIIcons(ascii))
}

// clientOptions enables the response cache unless disabled; a missing user
// cache dir silently runs uncached.
func clientOptions(noCache bool) []github.Option {
	if noCache {
		return nil
	}
	dir, err := github.DefaultCacheDir()
	if err != nil {
		return nil
	}
	return []github.Option{github.WithCache(dir, cacheTTL)}
}

const cacheTTL = time.Duration(github.DefaultCacheTTL)

func asciiFromEnv() bool { return envTruthy("WORTHY_ASCII") }

func envTruthy(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
