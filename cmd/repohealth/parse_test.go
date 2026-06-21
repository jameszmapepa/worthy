package main

import "testing"

func TestParseRepoArg(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"plain owner/repo", "charmbracelet/bubbletea", "charmbracelet", "bubbletea", false},
		{"https url", "https://github.com/charmbracelet/bubbletea", "charmbracelet", "bubbletea", false},
		{"http url", "http://github.com/charmbracelet/bubbletea", "charmbracelet", "bubbletea", false},
		{"bare github.com url", "github.com/charmbracelet/bubbletea", "charmbracelet", "bubbletea", false},
		{"url with trailing slash", "https://github.com/charmbracelet/bubbletea/", "charmbracelet", "bubbletea", false},
		{"url with .git suffix", "https://github.com/charmbracelet/bubbletea.git", "charmbracelet", "bubbletea", false},
		{"url with extra path segments", "https://github.com/charmbracelet/bubbletea/tree/main", "charmbracelet", "bubbletea", false},
		{"surrounding whitespace", "  charmbracelet/bubbletea  ", "charmbracelet", "bubbletea", false},

		{"hyphenated owner, dotted repo accepted", "o-w-ner/re.po-1", "o-w-ner", "re.po-1", false},

		{"empty", "", "", "", true},
		{"single segment", "charmbracelet", "", "", true},
		{"missing repo", "charmbracelet/", "", "", true},
		{"missing owner", "/bubbletea", "", "", true},
		{"not github host", "https://gitlab.com/a/b", "", "", true},
		{"whitespace only", "   ", "", "", true},

		// Charset / injection guards (validate at the boundary).
		{"parent dir traversal", "owner/..", "", "", true},
		{"current dir", "owner/.", "", "", true},
		{"query injection", "owner/r?x=1", "", "", true},
		{"command injection", "owner/r;rm -rf", "", "", true},
		{"space in repo", "owner/r space", "", "", true},
		{"owner starts with dash", "-bad/r", "", "", true},
		{"owner with dot rejected", "ow.ner/repo", "", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			owner, repo, err := parseRepoArg(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseRepoArg(%q) = (%q,%q,nil), want error", tc.in, owner, repo)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRepoArg(%q) unexpected error: %v", tc.in, err)
			}
			if owner != tc.wantOwner || repo != tc.wantRepo {
				t.Errorf("parseRepoArg(%q) = (%q,%q), want (%q,%q)",
					tc.in, owner, repo, tc.wantOwner, tc.wantRepo)
			}
		})
	}
}
