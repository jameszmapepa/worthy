package main

import (
	"bytes"
	"strings"
	"testing"
)

func runQuiet(args []string) (string, error) {
	var out, errb bytes.Buffer
	err := run(args, &out, &errb, true)
	return out.String(), err
}

func TestRunArgValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"no args", nil},
		{"too many args", []string{"a/b", "c/d"}},
		{"invalid repo arg", []string{"not-a-repo"}},
		{"non-github host", []string{"https://gitlab.com/a/b"}},
		{"flag only, no repo", []string{"--ascii"}},
		{"two positionals with a flag still errors", []string{"--ascii", "a/b", "c/d"}},
		{"unknown flag", []string{"--bogus", "a/b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := runQuiet(tc.args); err == nil {
				t.Errorf("run(%v) = nil, want error", tc.args)
			}
		})
	}
}

func TestParseArgsFlags(t *testing.T) {
	o, err := parseArgs([]string{"--json", "--no-cache", "-a", "owner/repo"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.json || !o.noCache || !o.ascii || o.target != "owner/repo" {
		t.Errorf("parsed = %+v", o)
	}
	o, err = parseArgs([]string{"owner/repo", "--plain", "--no-ascii"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.plain || o.ascii || o.json {
		t.Errorf("parsed = %+v", o)
	}
}

func TestSelectMode(t *testing.T) {
	cases := []struct {
		o    options
		tty  bool
		want mode
	}{
		{options{}, true, modeTUI},
		{options{}, false, modePlain},
		{options{plain: true}, true, modePlain},
		{options{json: true}, false, modeJSON},
		{options{json: true, plain: true}, true, modeJSON},
	}
	for _, c := range cases {
		if got := selectMode(c.o, c.tty); got != c.want {
			t.Errorf("selectMode(%+v, tty=%v) = %v, want %v", c.o, c.tty, got, c.want)
		}
	}
}

func TestHelpAndVersionExitCleanly(t *testing.T) {
	out, err := runQuiet([]string{"--help"})
	if err != nil || !strings.Contains(out, "usage: worthy") {
		t.Errorf("--help: err=%v out=%q", err, out)
	}
	out, err = runQuiet([]string{"--version"})
	if err != nil || !strings.HasPrefix(out, "worthy ") {
		t.Errorf("--version: err=%v out=%q", err, out)
	}
}

func TestEnvTruthy(t *testing.T) {
	for _, v := range []string{"1", "true", "YES", "on"} {
		t.Setenv("WORTHY_ASCII", v)
		if !envTruthy("WORTHY_ASCII") {
			t.Errorf("WORTHY_ASCII=%q should enable ascii mode", v)
		}
	}
	for _, v := range []string{"", "0", "no", "off", "garbage"} {
		t.Setenv("WORTHY_ASCII", v)
		if envTruthy("WORTHY_ASCII") {
			t.Errorf("WORTHY_ASCII=%q should NOT enable ascii mode", v)
		}
	}
}

func TestClientOptionsHonourNoCache(t *testing.T) {
	if got := clientOptions(true); got != nil {
		t.Errorf("--no-cache should yield no client options, got %d", len(got))
	}
	if got := clientOptions(false); len(got) != 1 {
		t.Errorf("default should enable the cache, got %d options", len(got))
	}
}

func TestNoCacheFlagIsAccepted(t *testing.T) {
	_, err := runQuiet([]string{"--no-cache"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Errorf("--no-cache alone should fail with usage, got %v", err)
	}
}
