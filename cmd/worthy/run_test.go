package main

import "testing"

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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := run(tc.args); err == nil {
				t.Errorf("run(%v) = nil, want error", tc.args)
			}
		})
	}
}

func TestAsciiFlagParsing(t *testing.T) {
	for _, args := range [][]string{
		{"--ascii", "owner/repo"},
		{"owner/repo", "-a"},
		{"--no-ascii", "owner/repo"},
	} {
		positional := make([]string, 0, len(args))
		for _, a := range args {
			switch a {
			case "--ascii", "-a", "--no-ascii":
			default:
				positional = append(positional, a)
			}
		}
		if len(positional) != 1 || positional[0] != "owner/repo" {
			t.Errorf("args %v: positional = %v; want exactly [owner/repo]", args, positional)
		}
	}
}

func TestAsciiFromEnv(t *testing.T) {
	for _, v := range []string{"1", "true", "YES", "on"} {
		t.Setenv("WORTHY_ASCII", v)
		if !asciiFromEnv() {
			t.Errorf("WORTHY_ASCII=%q should enable ascii mode", v)
		}
	}
	for _, v := range []string{"", "0", "no", "off", "garbage"} {
		t.Setenv("WORTHY_ASCII", v)
		if asciiFromEnv() {
			t.Errorf("WORTHY_ASCII=%q should NOT enable ascii mode", v)
		}
	}
}
