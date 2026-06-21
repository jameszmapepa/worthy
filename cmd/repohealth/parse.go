package main

import (
	"fmt"
	"regexp"
	"strings"
)

// ownerPattern matches a valid GitHub login: alphanumerics and internal
// hyphens only (no leading/trailing hyphen, no dots or underscores).
var ownerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)

// repoPattern matches a valid GitHub repository name: alphanumerics plus dot,
// underscore, and hyphen. The bare "." and ".." names are rejected separately
// as path-traversal guards.
var repoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// parseRepoArg extracts an owner and repo from a single command-line argument.
// It accepts three forms:
//
//	owner/repo
//	github.com/owner/repo
//	https://github.com/owner/repo   (http/https, optional trailing slash,
//	                                 optional .git suffix, extra path segments)
//
// Surrounding whitespace is trimmed. Any host other than github.com is
// rejected, as is a missing owner or repo.
func parseRepoArg(arg string) (owner, repo string, err error) {
	s := strings.TrimSpace(arg)
	if s == "" {
		return "", "", fmt.Errorf("empty repository argument")
	}

	// Normalize URL forms down to the "owner/repo[/...]" path.
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	if rest, ok := strings.CutPrefix(s, "github.com/"); ok {
		s = rest
	} else if strings.Contains(s, "://") || hasHost(s) {
		// A scheme or host we did not strip means a non-github source.
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

// validateOwnerRepo enforces the GitHub login and repository-name charsets at
// the trust boundary, rejecting path traversal (".", "..") and any character
// that could inject query parameters, shell metacharacters, or extra path
// segments downstream.
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

// hasHost reports whether the first path segment looks like a hostname (e.g.
// "gitlab.com") rather than a GitHub owner. A dot before the first slash is the
// signal — GitHub owners never contain dots.
func hasHost(s string) bool {
	first := s
	if i := strings.IndexByte(s, '/'); i >= 0 {
		first = s[:i]
	}
	return strings.Contains(first, ".")
}
