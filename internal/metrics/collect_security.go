package metrics

import (
	"bytes"
	"context"
	"strings"

	"github.com/jameszmapepa/repo-health/internal/github"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
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
//   - needsPartial is true only when ZERO files could be fetched (total
//     failure); the caller records "workflow_safety" in its own partial slot.
//
// File fetches fan out under the shared semaphore and gctx. Per-file
// non-context failures are silently skipped; a context cancellation/deadline
// is propagated as the returned error so the caller can abort.
func processWorkflows(
	gctx context.Context,
	c *github.Client,
	owner, repo string,
	sem *semaphore.Weighted,
	workflows []github.Workflow,
) (hasCI, usesPRT, fetched, needsPartial bool, err error) {
	for _, wf := range workflows {
		if wf.State == "active" {
			hasCI = true
		}
	}

	if len(workflows) == 0 {
		// Nothing to fetch; WorkflowsFetched stays false but no partial entry —
		// the repo simply has no workflows.
		return hasCI, false, false, false, nil
	}

	// Indexed per-file results; each goroutine writes only its own slot.
	bodies := make([][]byte, len(workflows))

	g, ictx := errgroup.WithContext(gctx)
	// ceiling: the shared semaphore already bounds concurrent HTTP calls
	// globally; SetLimit additionally bounds goroutine CREATION so a repo
	// listing very many workflows cannot spawn an unbounded number of parked
	// goroutines waiting on the semaphore.
	g.SetLimit(maxConcurrency)
	for i, wf := range workflows {
		g.Go(func() error {
			return withCall(ictx, sem, func() error {
				body, fetchErr := c.FileContent(ictx, owner, repo, wf.Path)
				if fetchErr != nil {
					if isContextError(fetchErr) {
						return fetchErr
					}
					return nil // swallow per-file failure
				}
				bodies[i] = body
				return nil
			})
		})
	}
	if waitErr := g.Wait(); waitErr != nil {
		return hasCI, false, false, false, waitErr
	}

	for _, body := range bodies {
		if body == nil {
			continue
		}
		fetched = true
		if bytes.Contains(body, []byte("pull_request_target")) { // A6: avoid string copy
			usesPRT = true
		}
	}

	if !fetched {
		// Could not fetch any file — safety state is unknown.
		return hasCI, false, false, true, nil
	}
	return hasCI, usesPRT, true, false, nil
}
