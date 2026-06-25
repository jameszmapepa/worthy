package metrics

import (
	"bytes"
	"context"
	"strings"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/jameszmapepa/worthy/internal/github"
)

var signatureExts = []string{".asc", ".sig", ".sigstore", ".intoto.jsonl"}

func hasSignatureExt(name string) bool {
	for _, ext := range signatureExts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

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
		return hasCI, false, false, false, nil
	}

	bodies := make([][]byte, len(workflows))

	g, ictx := errgroup.WithContext(gctx)

	g.SetLimit(maxConcurrency)
	for i, wf := range workflows {
		g.Go(func() error {
			return withCall(ictx, sem, func() error {
				body, fetchErr := c.FileContent(ictx, owner, repo, wf.Path)
				if fetchErr != nil {
					if isContextError(fetchErr) {
						return fetchErr
					}
					return nil
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
		if bytes.Contains(body, []byte("pull_request_target")) {
			usesPRT = true
		}
	}

	if !fetched {
		return hasCI, false, false, true, nil
	}
	return hasCI, usesPRT, true, false, nil
}
