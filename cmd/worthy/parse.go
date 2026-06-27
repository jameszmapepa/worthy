package main

import (
	"fmt"
	"regexp"
	"strings"
)

var ownerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func parseRepoArg(arg string) (owner, repo string, err error) {
	s := strings.TrimSpace(arg)
	if s == "" {
		return "", "", fmt.Errorf("empty repository argument")
	}

	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if rest, ok := strings.CutPrefix(s, "github.com/"); ok {
		s = rest
	} else if strings.Contains(s, "://") || hasHost(s) {
		return "", "", fmt.Errorf("only github.com repositories are supported: %q", arg)
	}

	s = strings.TrimSuffix(s, "/")
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("expected owner/repo, got %q", arg)
	}

	owner = parts[0]
	repo = strings.TrimSuffix(parts[1], ".git")
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("expected owner/repo, got %q", arg)
	}
	if err := validateOwnerRepo(owner, repo); err != nil {
		return "", "", err
	}
	return owner, repo, nil
}

// validateOwnerRepo guards the trust boundary: it rejects path traversal and
// any character that could inject params or path segments into downstream URLs.
func validateOwnerRepo(owner, repo string) error {
	if !ownerPattern.MatchString(owner) {
		return fmt.Errorf("invalid owner %q: must be alphanumerics and internal hyphens", owner)
	}
	if repo == "." || repo == ".." {
		return fmt.Errorf("invalid repo %q: path traversal not allowed", repo)
	}
	if !repoPattern.MatchString(repo) {
		return fmt.Errorf("invalid repo %q: must be alphanumerics, '.', '_', or '-'", repo)
	}
	return nil
}

func hasHost(s string) bool {
	first, _, _ := strings.Cut(s, "/")
	return strings.Contains(first, ".")
}
