package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// withFakeHome redirects HOME so tests don't touch the real ~/.local/state.
func withFakeHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestSaveLoadRoundTrip(t *testing.T) {
	withFakeHome(t)

	in := Empty()
	in.MepVersion = "0.0.1-test"
	in.Skills["humanizer"] = Skill{
		Kind:    "managed",
		Version: "2.3.0",
		Source:  "local:/tmp/x",
		SHA256:  "abc",
		Targets: map[string]TargetRecord{
			"claude": {Files: []FileRecord{{Path: "/x/humanizer.md", SHA256: "deadbeef"}}},
		},
		InstalledAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := Save(in); err != nil {
		t.Fatalf("save: %v", err)
	}

	out, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out.MepVersion != "0.0.1-test" {
		t.Errorf("version: got %q", out.MepVersion)
	}
	if got := out.Skills["humanizer"].Version; got != "2.3.0" {
		t.Errorf("humanizer version: got %q", got)
	}
}

func TestLoadMissingFileReturnsEmpty(t *testing.T) {
	withFakeHome(t)
	s, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(s.Skills) != 0 {
		t.Errorf("expected empty, got %v", s.Skills)
	}
	if s.ManifestSchema != ManifestSchema {
		t.Errorf("schema: got %d", s.ManifestSchema)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	// Verifies no leftover temp files after a successful save.
	withFakeHome(t)
	dir, err := StateDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(Empty()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" && e.Name() != "state.json" {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestAppendHistory(t *testing.T) {
	withFakeHome(t)
	if err := AppendHistory(HistoryEvent{Op: "install", Skill: "humanizer", Version: "2.3.0"}); err != nil {
		t.Fatal(err)
	}
	if err := AppendHistory(HistoryEvent{Op: "uninstall", Skill: "pdf-compare"}); err != nil {
		t.Fatal(err)
	}
	p, _ := historyPath()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// Two newline-terminated JSON lines.
	count := 0
	for _, c := range b {
		if c == '\n' {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 lines, got %d (content: %s)", count, string(b))
	}
}

func TestLockReleaseRoundTrip(t *testing.T) {
	withFakeHome(t)
	f, err := Lock()
	if err != nil {
		t.Fatalf("lock: %v", err)
	}
	if err := Release(f); err != nil {
		t.Fatalf("release: %v", err)
	}
	// Should be able to reacquire.
	f2, err := Lock()
	if err != nil {
		t.Fatalf("re-lock: %v", err)
	}
	_ = Release(f2)
}
