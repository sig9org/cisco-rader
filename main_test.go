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
	for _, want := range []string{"usage: cisco-rader [flags]", "-p, -profile string", "-v, -version", "-h, -help"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("help does not contain %q", want)
		}
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
