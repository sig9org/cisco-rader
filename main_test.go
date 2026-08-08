package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	for _, want := range []string{"usage: cisco-rader [flags]", "-profile string", "-separate", "-v, -version", "-h, -help"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("help does not contain %q", want)
		}
	}
	if strings.Contains(stdout.String(), "-p,") {
		t.Errorf("help still advertises removed -p option")
	}
}

func TestRunRejectsRemovedShortProfileOption(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-p", "webex"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -p") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunHelpAlignsFlagDescriptions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	descriptions := []string{
		"site configuration file",
		"chat configuration file",
		"chat configuration profile",
		"send each software update",
		"run Chrome or Chromium",
		"browser User-Agent",
		"release information retrieval timeout",
		"do not send chat notifications",
		"do not write the derived",
		"show the planned operation",
		"suppress normal stdout messages",
		"print timestamped debug tracing",
		"update cisco-rader",
		"print version information",
		"show this help message",
	}
	wantColumn := -1
	for _, description := range descriptions {
		column := strings.Index(stdout.String(), description)
		if column < 0 {
			t.Fatalf("help does not contain %q", description)
		}
		lineStart := strings.LastIndex(stdout.String()[:column], "\n") + 1
		column -= lineStart
		if wantColumn == -1 {
			wantColumn = column
		} else if column != wantColumn {
			t.Errorf("%q starts at column %d, want %d", description, column, wantColumn)
		}
	}
}

func TestRunRejectsNonPositiveTimeout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-timeout", "0s"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "-timeout must be greater than zero") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunDryRunDoesNotCreateState(t *testing.T) {
	dir := t.TempDir()
	sitePath := filepath.Join(dir, "custom.yml")
	data := []byte("sites:\n  - name: Test\n    url: https://software.cisco.com/test\n")
	if err := os.WriteFile(sitePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-site", sitePath, "-dryrun"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "custom_state.yml")); !os.IsNotExist(err) {
		t.Fatalf("dry run created state: %v", err)
	}
	if !strings.Contains(stdout.String(), "would check 1 site(s)") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestRunDryRunReportsBrowserOptions(t *testing.T) {
	dir := t.TempDir()
	sitePath := filepath.Join(dir, "custom.yml")
	data := []byte("sites:\n  - name: Test\n    url: https://software.cisco.com/test\n")
	if err := os.WriteFile(sitePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{"-site", sitePath, "-dryrun", "-headless", "-user-agent", "Custom Agent", "-timeout", "1m30s"}
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	want := "browser mode=headless, timeout=1m30s, custom User-Agent=true"
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("stdout does not contain %q: %s", want, stdout.String())
	}
}

func TestChatToolDisplayName(t *testing.T) {
	tests := map[string]string{
		"teams":  "Microsoft Teams",
		"webex":  "Cisco Webex",
		"slack":  "Slack",
		"custom": "custom",
	}
	for input, want := range tests {
		if got := chatToolDisplayName(input); got != want {
			t.Errorf("chatToolDisplayName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDebugOverridesSilent(t *testing.T) {
	dir := t.TempDir()
	sitePath := filepath.Join(dir, "custom.yml")
	if err := os.WriteFile(sitePath, []byte("sites:\n  - url: https://software.cisco.com/test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-site", sitePath, "-dryrun", "-silent", "-debug"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "[DEBUG]") || !strings.Contains(stdout.String(), "Dry run") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
