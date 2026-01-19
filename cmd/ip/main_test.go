package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/example/instapaper-cli/internal/instapaper"
)

func runCmd(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out bytes.Buffer
	var err bytes.Buffer
	code := run(args, &out, &err)
	return code, out.String(), err.String()
}

func tempConfigArg(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	return []string{"--config", cfg}
}

func TestHelpAndVersion(t *testing.T) {
	args := append([]string{"ip"}, tempConfigArg(t)...)
	code, out, _ := runCmd(t, append(args, "--help")...)
	if code != 0 {
		t.Fatalf("help exit=%d", code)
	}
	if !strings.Contains(out, "Usage:") {
		t.Fatalf("help output missing Usage")
	}

	code, out, _ = runCmd(t, append(args, "--version")...)
	if code != 0 {
		t.Fatalf("version exit=%d", code)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("version output empty")
	}
}

func TestHelpSubcommand(t *testing.T) {
	args := append([]string{"ip"}, tempConfigArg(t)...)
	code, out, _ := runCmd(t, append(args, "help", "list")...)
	if code != 0 {
		t.Fatalf("help list exit=%d", code)
	}
	if !strings.Contains(out, "ip list") {
		t.Fatalf("help list output unexpected: %s", out)
	}
}

func TestInvalidFormat(t *testing.T) {
	args := append([]string{"ip"}, tempConfigArg(t)...)
	code, _, errOut := runCmd(t, append(args, "--format", "nope", "config", "path")...)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
	if !strings.Contains(errOut, "invalid --format") {
		t.Fatalf("expected format error, got: %s", errOut)
	}
}

func TestUnknownCommand(t *testing.T) {
	args := append([]string{"ip"}, tempConfigArg(t)...)
	code, _, errOut := runCmd(t, append(args, "wat")...)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
	if !strings.Contains(errOut, "unknown command") {
		t.Fatalf("expected unknown command error, got: %s", errOut)
	}
}

func TestConfigPath(t *testing.T) {
	args := append([]string{"ip"}, tempConfigArg(t)...)
	code, out, _ := runCmd(t, append(args, "config", "path")...)
	if code != 0 {
		t.Fatalf("config path exit=%d", code)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("config path output empty")
	}
}

func TestExitCodeForAPIError(t *testing.T) {
	cases := []struct {
		code int
		want int
	}{
		{code: 1040, want: 10},
		{code: 1041, want: 11},
		{code: 1042, want: 12},
		{code: 1240, want: 13},
		{code: 1500, want: 14},
		{code: 9999, want: 1},
	}
	for _, tc := range cases {
		got := exitCodeForError(&instapaper.APIError{Code: tc.code})
		if got != tc.want {
			t.Fatalf("code %d: got %d want %d", tc.code, got, tc.want)
		}
	}
}
