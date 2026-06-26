package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A1: ANSI/OSC sequences in API-sourced fields must be stripped before TUI rendering.
func TestCollect_ANSIStripped_FromRepoPresentationFields(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")

	const dirtyDesc = "\x1b[2JHello World\x1b]8;;https://evil.com\x1b\\click me\x1b]8;;\x1b\\"
	const dirtyLang = "\x1b[31mGo\x1b[0m"
	const dirtyLicense = "\x1b[1mMIT\x1b[0m"

	descJSON, _ := json.Marshal(dirtyDesc)
	langJSON, _ := json.Marshal(dirtyLang)
	licenseJSON, _ := json.Marshal(dirtyLicense)

	dirtyRepoJSON := fmt.Sprintf(`{
		"full_name":"acme/widget",
		"description":%s,
		"language":%s,
		"stargazers_count":0,"subscribers_count":0,
		"forks_count":0,"open_issues_count":0,"archived":false,"disabled":false,
		"fork":false,
		"pushed_at":"2026-06-01T00:00:00Z","created_at":"2024-01-01T00:00:00Z",
		"default_branch":"main",
		"license":{"spdx_id":%s,"name":"MIT License"}
	}`, descJSON, langJSON, licenseJSON)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/acme/widget":
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, dirtyRepoJSON)
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `[]`)
		}
	}))
	defer srv.Close()

	got, err := Collect(context.Background(), client(srv), "acme", "widget", now)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if strings.ContainsRune(got.Description, '\x1b') {
		t.Errorf("Description contains ESC after sanitization: %q", got.Description)
	}
	if strings.ContainsRune(got.Language, '\x1b') {
		t.Errorf("Language contains ESC after sanitization: %q", got.Language)
	}
	if strings.ContainsRune(got.LicenseSPDX, '\x1b') {
		t.Errorf("LicenseSPDX contains ESC after sanitization: %q", got.LicenseSPDX)
	}

	if !strings.Contains(got.Description, "Hello World") {
		t.Errorf("Description %q: visible text 'Hello World' must survive stripping", got.Description)
	}
	if !strings.Contains(got.Language, "Go") {
		t.Errorf("Language %q: visible text 'Go' must survive stripping", got.Language)
	}
	if !strings.Contains(got.LicenseSPDX, "MIT") {
		t.Errorf("LicenseSPDX %q: visible text 'MIT' must survive stripping", got.LicenseSPDX)
	}
}
