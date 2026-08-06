package state

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/sig9org/cisco-rader/internal/model"
)

func TestLoadMissingReturnsEmpty(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.yml"))
	if err != nil || got.Sites == nil || len(got.Sites) != 0 {
		t.Fatalf("Load missing = %#v, %v", got, err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	want := File{Sites: map[string]model.Snapshot{
		"https://example.com": {
			ProductName: "Product", Suggested: []string{"1.0"}, Latest: []string{"1.1"},
			FetchedAt: time.Date(2026, 8, 6, 1, 2, 3, 0, time.UTC),
		},
	}}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}
