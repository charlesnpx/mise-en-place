package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charlesnpx/mise-en-place/internal/config"
	"github.com/charlesnpx/mise-en-place/internal/state"
)

// withFakeHome redirects HOME so install tests don't touch the real
// ~/.local/state or ~/.claude directories.
func withFakeHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

// writeFixtureSkill creates a minimal managed skill on disk and returns the
// path to its directory.
func writeFixtureSkill(t *testing.T, root, name, claudePath, codexDirPath string) string {
	t.Helper()
	skillDir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(skillDir, "payload", "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(skillDir, "payload", "codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "payload", "claude", filepath.Base(claudePath)),
		[]byte("# claude payload\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "payload", "codex", "SKILL.md"),
		[]byte("# codex payload\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `name: ` + name + `
version: 0.1.0
manifest_schema: 1
min_installer: "0.0.0"
targets:
  claude:
    type: command
    payload: payload/claude/` + filepath.Base(claudePath) + `
    install_to: ` + claudePath + `
  codex:
    type: skill_dir
    payload: payload/codex/
    install_to: ` + codexDirPath + `
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return skillDir
}

func TestInstall_Managed_DualTarget(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	claudePath := filepath.Join(home, ".claude", "commands", "fixture.md")
	codexDir := filepath.Join(home, ".codex", "skills", "fixture/")
	writeFixtureSkill(t, skillsRoot, "fixture", claudePath, codexDir)

	reg := &config.Registry{Managed: []string{"fixture"}}
	opts := Options{
		Target:           "all",
		RunningInstaller: "0.1.0",
		ManifestSchema:   1,
		SkillsRoot:       skillsRoot,
	}

	if err := One("fixture", reg, opts); err != nil {
		t.Fatalf("install: %v", err)
	}

	// Both target files exist.
	if _, err := os.Stat(claudePath); err != nil {
		t.Errorf("claude payload missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(codexDir, "SKILL.md")); err != nil {
		t.Errorf("codex payload missing: %v", err)
	}

	// state.json records the install.
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	sk, ok := s.Skills["fixture"]
	if !ok {
		t.Fatalf("fixture not in state.json")
	}
	if sk.Version != "0.1.0" {
		t.Errorf("version: %q", sk.Version)
	}
	if len(sk.Targets) != 2 {
		t.Errorf("targets: %d", len(sk.Targets))
	}
	if len(sk.Targets["claude"].Files) != 1 || sk.Targets["claude"].Files[0].Path != claudePath {
		t.Errorf("claude file record: %+v", sk.Targets["claude"])
	}
}

func TestInstall_TargetFilter(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	claudePath := filepath.Join(home, ".claude", "commands", "fixture.md")
	codexDir := filepath.Join(home, ".codex", "skills", "fixture/")
	writeFixtureSkill(t, skillsRoot, "fixture", claudePath, codexDir)

	reg := &config.Registry{Managed: []string{"fixture"}}
	opts := Options{
		Target:           "claude",
		RunningInstaller: "0.1.0",
		ManifestSchema:   1,
		SkillsRoot:       skillsRoot,
	}
	if err := One("fixture", reg, opts); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := os.Stat(claudePath); err != nil {
		t.Errorf("claude payload missing")
	}
	if _, err := os.Stat(filepath.Join(codexDir, "SKILL.md")); err == nil {
		t.Errorf("codex payload should NOT have been installed")
	}
}

func TestInstall_RefusesUnmanagedFile(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	claudePath := filepath.Join(home, ".claude", "commands", "fixture.md")
	codexDir := filepath.Join(home, ".codex", "skills", "fixture/")
	writeFixtureSkill(t, skillsRoot, "fixture", claudePath, codexDir)

	// Pre-place a hand-copied file at the install destination.
	if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudePath, []byte("hand-copied\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &config.Registry{Managed: []string{"fixture"}}
	opts := Options{
		Target:           "claude",
		RunningInstaller: "0.1.0",
		ManifestSchema:   1,
		SkillsRoot:       skillsRoot,
	}
	err := One("fixture", reg, opts)
	if err == nil {
		t.Fatalf("expected install to refuse pre-existing unmanaged file")
	}
	if !strings.Contains(err.Error(), "unmanaged") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstall_BackupRescuesUnmanagedFile(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	claudePath := filepath.Join(home, ".claude", "commands", "fixture.md")
	codexDir := filepath.Join(home, ".codex", "skills", "fixture/")
	writeFixtureSkill(t, skillsRoot, "fixture", claudePath, codexDir)

	if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudePath, []byte("hand-copied\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &config.Registry{Managed: []string{"fixture"}}
	opts := Options{
		Target:           "claude",
		Backup:           true,
		RunningInstaller: "0.1.0",
		ManifestSchema:   1,
		SkillsRoot:       skillsRoot,
	}
	if err := One("fixture", reg, opts); err != nil {
		t.Fatalf("install with backup: %v", err)
	}
	// The new file is in place.
	body, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "claude payload") {
		t.Errorf("expected new payload, got: %q", string(body))
	}
	// A backup with the .mise-en-place.bak. infix exists.
	entries, err := os.ReadDir(filepath.Dir(claudePath))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), ".mise-en-place.bak.") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a *.mise-en-place.bak.* file alongside %s", claudePath)
	}
}

func TestInstall_RefusesCrossSkillCollision(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	claudePath := filepath.Join(home, ".claude", "commands", "fixture.md")
	codexDir := filepath.Join(home, ".codex", "skills", "fixture/")
	writeFixtureSkill(t, skillsRoot, "alpha", claudePath, codexDir)
	// Second fixture wants the *same* claude install path.
	writeFixtureSkill(t, skillsRoot, "beta", claudePath, filepath.Join(home, ".codex", "skills", "beta/"))

	reg := &config.Registry{Managed: []string{"alpha", "beta"}}
	opts := Options{
		Target:           "all",
		RunningInstaller: "0.1.0",
		ManifestSchema:   1,
		SkillsRoot:       skillsRoot,
	}
	if err := One("alpha", reg, opts); err != nil {
		t.Fatalf("install alpha: %v", err)
	}
	err := One("beta", reg, opts)
	if err == nil {
		t.Fatalf("expected collision error")
	}
	if !strings.Contains(err.Error(), "collision") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestInstall_UninstallRoundTrip(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	claudePath := filepath.Join(home, ".claude", "commands", "fixture.md")
	codexDir := filepath.Join(home, ".codex", "skills", "fixture/")
	writeFixtureSkill(t, skillsRoot, "fixture", claudePath, codexDir)

	reg := &config.Registry{Managed: []string{"fixture"}}
	opts := Options{
		Target:           "all",
		RunningInstaller: "0.1.0",
		ManifestSchema:   1,
		SkillsRoot:       skillsRoot,
	}
	if err := One("fixture", reg, opts); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := Uninstall("fixture"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(claudePath); err == nil {
		t.Errorf("claude file should be removed")
	}
	if _, err := os.Stat(filepath.Join(codexDir, "SKILL.md")); err == nil {
		t.Errorf("codex file should be removed")
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Skills["fixture"]; ok {
		t.Errorf("state should no longer record fixture")
	}
}

func TestInstall_LeavesLocallyModifiedFileOnUninstall(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	claudePath := filepath.Join(home, ".claude", "commands", "fixture.md")
	codexDir := filepath.Join(home, ".codex", "skills", "fixture/")
	writeFixtureSkill(t, skillsRoot, "fixture", claudePath, codexDir)

	reg := &config.Registry{Managed: []string{"fixture"}}
	opts := Options{
		Target:           "claude",
		RunningInstaller: "0.1.0",
		ManifestSchema:   1,
		SkillsRoot:       skillsRoot,
	}
	if err := One("fixture", reg, opts); err != nil {
		t.Fatal(err)
	}
	// Locally edit the installed file.
	if err := os.WriteFile(claudePath, []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall("fixture"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	// The edited file should still be there.
	body, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("expected locally-modified file to survive uninstall: %v", err)
	}
	if string(body) != "edited\n" {
		t.Errorf("file contents changed: %q", string(body))
	}
}
