package logx

import (
	"bytes"
	"strings"
	"testing"
)

func TestSeverityColors(t *testing.T) {
	var out, stderr bytes.Buffer
	logger := Logger{Out: &out, Err: &stderr}
	logger.Infof("normal")
	logger.Warnf("warning")
	logger.Errorf("error")
	if out.String() != "normal\n" {
		t.Fatalf("normal output = %q", out.String())
	}
	if !strings.Contains(stderr.String(), orange+"warning"+reset) ||
		!strings.Contains(stderr.String(), red+"error"+reset) {
		t.Fatalf("severity output = %q", stderr.String())
	}
}

func TestDebugOverridesSilent(t *testing.T) {
	var out bytes.Buffer
	logger := Logger{Out: &out, Silent: true, Debug: true}
	logger.Infof("normal")
	logger.Debugf("details")
	if !strings.Contains(out.String(), "normal") || !strings.Contains(out.String(), "[DEBUG] details") ||
		!strings.Contains(out.String(), gray) {
		t.Fatalf("output = %q", out.String())
	}
}
