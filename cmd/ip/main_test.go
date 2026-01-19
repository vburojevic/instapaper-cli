package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vburojevic/instapaper-cli/internal/config"
	"github.com/vburojevic/instapaper-cli/internal/instapaper"
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

func writeConfig(t *testing.T, path string, cfg *config.Config) {
	t.Helper()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save config: %v", err)
	}
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

func TestConfigShowTable(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.json")
	cfg := config.DefaultConfig()
	cfg.ConsumerKey = "ck"
	cfg.ConsumerSecret = "cs"
	writeConfig(t, cfgPath, cfg)

	code, out, _ := runCmd(t, "ip", "--config", cfgPath, "--format", "table", "config", "show")
	if code != 0 {
		t.Fatalf("config show exit=%d", code)
	}
	if !strings.Contains(out, "api_base") {
		t.Fatalf("expected api_base in output: %s", out)
	}
}

func TestConfigShowJSON(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.json")
	cfg := config.DefaultConfig()
	writeConfig(t, cfgPath, cfg)

	code, out, _ := runCmd(t, "ip", "--config", cfgPath, "--json", "config", "show")
	if code != 0 {
		t.Fatalf("config show json exit=%d", code)
	}
	if !strings.Contains(out, "\"api_base\"") {
		t.Fatalf("expected json output, got: %s", out)
	}
}

func TestAuthStatusJSON(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.json")
	writeConfig(t, cfgPath, config.DefaultConfig())

	code, out, _ := runCmd(t, "ip", "--config", cfgPath, "--json", "auth", "status")
	if code != 0 {
		t.Fatalf("auth status json exit=%d", code)
	}
	if !strings.Contains(out, "\"logged_in\"") {
		t.Fatalf("expected json output, got: %s", out)
	}
}

func TestStderrJSON(t *testing.T) {
	args := append([]string{"ip"}, tempConfigArg(t)...)
	code, _, errOut := runCmd(t, append(args, "--stderr-json", "--format", "table", "list", "--limit", "-1")...)
	if code == 0 {
		t.Fatalf("expected error exit code")
	}
	if !strings.Contains(errOut, "\"error\"") {
		t.Fatalf("expected json error on stderr, got: %s", errOut)
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

func TestParseBoundSpec(t *testing.T) {
	bound, err := parseBoundSpec("123", "bookmark_id")
	if err != nil {
		t.Fatalf("parse bound: %v", err)
	}
	if bound.Field != "bookmark_id" || bound.Value != 123 {
		t.Fatalf("unexpected bound: %+v", bound)
	}

	bound, err = parseBoundSpec("time:2025-01-01T00:00:00Z", "bookmark_id")
	if err != nil {
		t.Fatalf("parse time bound: %v", err)
	}
	if bound.Field != "time" || bound.Value == 0 {
		t.Fatalf("unexpected time bound: %+v", bound)
	}
}

func TestFilterBookmarksByBounds(t *testing.T) {
	bookmarks := []instapaper.Bookmark{
		{BookmarkID: 1, Time: 100, ProgressTimestamp: 0},
		{BookmarkID: 2, Time: 200, ProgressTimestamp: 300},
	}
	since, err := parseUpdatedBound("150")
	if err != nil {
		t.Fatalf("parse updated bound: %v", err)
	}
	filtered := filterBookmarksByBounds(bookmarks, since, nil)
	if len(filtered) != 1 || int64(filtered[0].BookmarkID) != 2 {
		t.Fatalf("unexpected filtered result: %+v", filtered)
	}
}

func TestFilterBookmarksBySelect(t *testing.T) {
	bookmarks := []instapaper.Bookmark{
		{
			BookmarkID: 1,
			Title:      "Hello World",
			Starred:    instapaper.BoolInt(true),
			Tags:       []instapaper.Tag{{Name: "news"}},
		},
		{
			BookmarkID: 2,
			Title:      "Other",
			Starred:    instapaper.BoolInt(false),
			Tags:       []instapaper.Tag{{Name: "misc"}},
		},
	}
	filtered, err := filterBookmarksBySelect(bookmarks, "starred=1,tag~news")
	if err != nil {
		t.Fatalf("select filter: %v", err)
	}
	if len(filtered) != 1 || int64(filtered[0].BookmarkID) != 1 {
		t.Fatalf("unexpected select result: %+v", filtered)
	}
}
