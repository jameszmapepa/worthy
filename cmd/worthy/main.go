// Command worthy scores a public GitHub repository for maintenance health and contributor-friendliness.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"

	"github.com/jameszmapepa/worthy/internal/github"
	"github.com/jameszmapepa/worthy/internal/metrics"
	"github.com/jameszmapepa/worthy/internal/output"
	"github.com/jameszmapepa/worthy/internal/score"
	"github.com/jameszmapepa/worthy/internal/tui"
)

const usage = `usage: worthy [flags] <owner/repo | github.com/owner/repo | https://github.com/owner/repo>

  --json       print the score as JSON and exit
  --plain      print the scorecard without the interactive UI
               (the default when stdout is not a terminal)
  --ascii, -a  language tags instead of Nerd Font icons (WORTHY_ASCII=1)
  --no-cache   ignore and do not write the response cache (WORTHY_NO_CACHE=1)
  --version    print the version and exit
  --help, -h   this text`

type mode int

const (
	modeTUI mode = iota
	modePlain
	modeJSON
)

type options struct {
	ascii   bool
	noCache bool
	json    bool
	plain   bool
	version bool
	help    bool
	target  string
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, term.IsTerminal(os.Stdout.Fd())); err != nil {
		fmt.Fprintln(os.Stderr, "worthy:", err)
		os.Exit(2)
	}
}

func parseArgs(args []string) (options, error) {
	o := options{ascii: envTruthy("WORTHY_ASCII"), noCache: envTruthy("WORTHY_NO_CACHE")}
	positional := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "--ascii", "-a":
			o.ascii = true
		case "--no-ascii":
			o.ascii = false
		case "--no-cache":
			o.noCache = true
		case "--json":
			o.json = true
		case "--plain":
			o.plain = true
		case "--version", "-v":
			o.version = true
		case "--help", "-h":
			o.help = true
		default:
			if strings.HasPrefix(a, "-") {
				return o, fmt.Errorf("unknown flag %s\n%s", a, usage)
			}
			positional = append(positional, a)
		}
	}
	if o.help || o.version {
		return o, nil
	}
	if len(positional) != 1 {
		return o, errors.New(usage)
	}
	o.target = positional[0]
	return o, nil
}

// selectMode picks the output: explicit flags win, then a pipe gets plain text.
func selectMode(o options, stdoutIsTTY bool) mode {
	switch {
	case o.json:
		return modeJSON
	case o.plain || !stdoutIsTTY:
		return modePlain
	default:
		return modeTUI
	}
}

func run(args []string, stdout, stderr io.Writer, stdoutIsTTY bool) error {
	o, err := parseArgs(args)
	if err != nil {
		return err
	}
	if o.help {
		_, err = fmt.Fprintln(stdout, usage)
		return err
	}
	if o.version {
		_, err = fmt.Fprintln(stdout, "worthy", version())
		return err
	}
	owner, repo, err := parseRepoArg(o.target)
	if err != nil {
		return err
	}
	client := github.NewClient(clientOptions(o.noCache)...)
	ctx := context.Background()

	switch selectMode(o, stdoutIsTTY) {
	case modeTUI:
		return tui.Run(ctx, client, owner, repo, tui.WithASCIIIcons(o.ascii))
	case modeJSON:
		r, raw, err := collect(ctx, client, owner, repo, stderr)
		if err != nil {
			return err
		}
		return output.JSON(stdout, owner, repo, r, raw)
	default:
		r, raw, err := collect(ctx, client, owner, repo, stderr)
		if err != nil {
			return err
		}
		width := 100
		if stdoutIsTTY {
			if w, _, sizeErr := term.GetSize(os.Stdout.Fd()); sizeErr == nil && w > 0 {
				width = w
			}
		}
		page := tui.RenderPlain(owner, repo, r, raw, tui.PlainOptions{
			Width: width, ASCIIIcons: o.ascii, Authenticated: client.Authenticated(), Rate: client.RateInfo(),
		})
		// The colour-profile writer downsamples for the terminal and strips
		// styling entirely for pipes and NO_COLOR.
		_, err = colorprofile.NewWriter(stdout, os.Environ()).WriteString(page)
		return err
	}
}

const collectTimeout = 60 * time.Second

// collect scores the repo without a UI, narrating stages on stderr only when
// stderr is a terminal so logs and pipes stay clean.
func collect(ctx context.Context, client *github.Client, owner, repo string, stderr io.Writer) (score.Report, score.RawMetrics, error) {
	ctx, cancel := context.WithTimeout(ctx, collectTimeout)
	defer cancel()
	var opts []metrics.Option
	if f, ok := stderr.(*os.File); ok && term.IsTerminal(f.Fd()) {
		opts = append(opts, metrics.WithProgress(func(p metrics.Progress) {
			if p.State == metrics.StageRetrying {
				_, _ = fmt.Fprintf(stderr, "worthy: %s: GitHub is computing stats, retry %d\n", p.Stage, p.Attempt) //nolint:errcheck // narration only
			}
		}))
	}
	raw, err := metrics.Collect(ctx, client, owner, repo, time.Now(), opts...)
	if err != nil {
		var rl *github.RateLimitError
		if errors.As(err, &rl) {
			return score.Report{}, score.RawMetrics{}, fmt.Errorf("%w\nTip: set GITHUB_TOKEN to lift the limit to 5,000 requests/hour", err)
		}
		return score.Report{}, score.RawMetrics{}, err
	}
	return score.Evaluate(raw), raw, nil
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
	return []github.Option{github.WithCache(dir, github.DefaultCacheTTL)}
}

func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

func asciiFromEnv() bool { return envTruthy("WORTHY_ASCII") }

func envTruthy(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
