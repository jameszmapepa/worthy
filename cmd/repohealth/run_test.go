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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := run(tc.args); err == nil {
				t.Errorf("run(%v) = nil, want error", tc.args)
			}
		})
	}
}
