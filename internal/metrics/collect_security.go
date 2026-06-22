package metrics

import (
	"context"
	"strings"

	"github.com/jameszmapepa/repo-health/internal/github"
)

// signatureExts is the set of file-name suffixes that indicate a signed
// release asset per the spec.
var signatureExts = []string{".asc", ".sig", ".sigstore", ".intoto.jsonl"}

// hasSignatureExt reports whether name ends with a signature extension.
func hasSignatureExt(name string) bool {
	for _, ext := range signatureExts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// processWorkflows determines HasCI, UsesPullRequestTarget, and
// WorkflowsFetched by fetching each workflow file's content and scanning for
// the literal "pull_request_target" string.
//
// Per-file tolerance: a fetch failure (404, rate-limit, network) on an
// individual file is silently skipped — the scan continues with any files that
// could be fetched.
//   - WorkflowsFetched=true if at least one file was successfully fetched.
//   - UsesPullRequestTarget=true if any successfully-fetched file contained
//     the trigger literal.
//   - "workflow_safety" is appended to partial only when ZERO files could be
//     fetched (total failure).
func processWorkflows(
	ctx context.Context,
	c *github.Client,
	owner, repo string,
	workflows []github.Workflow,
	partial *[]string,
) (hasCI, usesPRT, fetched bool) {
	for _, wf := range workflows {
		if wf.State == "active" {
			hasCI = true
		}
	}

	if len(workflows) == 0 {
		// Nothing to fetch; WorkflowsFetched stays false but no partial entry —
		// the repo simply has no workflows.
		return hasCI, false, false
	}

	// Attempt to fetch each workflow file. Tolerate per-file failures.
	for _, wf := range workflows {
		body, err := c.FileContent(ctx, owner, repo, wf.Path)
		if err != nil {
			// Skip this file; continue scanning others.
			continue
		}
		fetched = true
		if strings.Contains(string(body), "pull_request_target") {
			usesPRT = true
		}
	}

	if !fetched {
		// Could not fetch any file — safety state is unknown.
		*partial = append(*partial, "workflow_safety")
		return hasCI, false, false
	}
	return hasCI, usesPRT, true
}
