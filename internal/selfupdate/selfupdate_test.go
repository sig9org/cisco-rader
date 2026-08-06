package selfupdate

import "testing"

func TestNormalizeVersion(t *testing.T) {
	tests := map[string]string{
		"dev":               "",
		"v1.2.3":            "v1.2.3",
		"v1.2.3-4-g1234567": "v1.2.3",
	}
	for input, want := range tests {
		if got := normalizeVersion(input); got != want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", input, got, want)
		}
	}
}
