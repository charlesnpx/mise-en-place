package install

import (
	"bytes"
	"io"
	"os"
	"os/exec"
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

func withNonInteractiveStdin(t *testing.T) {
	t.Helper()
	original := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = original
		_ = r.Close()
	})
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	restored := false
	defer func() {
		if !restored {
			os.Stdout = original
			_ = w.Close()
			_ = r.Close()
		}
	}()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = original
	restored = true
	defer r.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func writeFakeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFakePipx(t *testing.T, dir string) string {
	t.Helper()
	script := `#!/bin/sh
set -eu
echo "$@" >> "$HOME/pipx.log"
if [ "$1" = "install" ]; then
  mkdir -p "$PIPX_FAKE_BIN"
  printf '#!/bin/sh\nprintf fake-tool\\n\n' > "$PIPX_FAKE_BIN/$PIPX_FAKE_EXE"
  chmod 755 "$PIPX_FAKE_BIN/$PIPX_FAKE_EXE"
elif [ "$1" = "uninstall" ]; then
  rm -f "$PIPX_FAKE_BIN/$PIPX_FAKE_EXE"
fi
`
	return writeFakeExecutable(t, dir, "pipx", script)
}

func withFakePipx(t *testing.T, home, executable string) string {
	t.Helper()
	bin := filepath.Join(home, "fake-bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakePipx(t, bin)
	t.Setenv("PIPX_FAKE_BIN", bin)
	t.Setenv("PIPX_FAKE_EXE", executable)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return bin
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

func TestInstallAll_SkipsExperimentalUnlessIncluded(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	stableCodexDir := filepath.Join(home, ".codex", "skills", "stable/")
	experimentalCodexDir := filepath.Join(home, ".codex", "skills", "experimental/")
	writeFixtureSkill(t, skillsRoot, "stable", filepath.Join(home, ".claude", "commands", "stable.md"), stableCodexDir)
	writeFixtureSkill(t, skillsRoot, "experimental", filepath.Join(home, ".claude", "commands", "experimental.md"), experimentalCodexDir)

	reg := &config.Registry{
		Managed:      []string{"stable", "experimental"},
		Experimental: []string{"experimental"},
	}
	opts := Options{
		Target:           "codex",
		RunningInstaller: "0.1.0",
		ManifestSchema:   1,
		SkillsRoot:       skillsRoot,
	}

	out := captureStdout(t, func() {
		if err := All(reg, opts); err != nil {
			t.Fatalf("default install: %v", err)
		}
	})
	if strings.Contains(out, "experimental skill") {
		t.Fatalf("default install should not warn for skipped experimental skill: %q", out)
	}
	if _, err := os.Stat(filepath.Join(stableCodexDir, "SKILL.md")); err != nil {
		t.Fatalf("stable skill should be installed by default: %v", err)
	}
	if _, err := os.Stat(filepath.Join(experimentalCodexDir, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("experimental skill should be skipped by default, stat err: %v", err)
	}

	opts.IncludeExperimental = true
	out = captureStdout(t, func() {
		if err := All(reg, opts); err != nil {
			t.Fatalf("install --all: %v", err)
		}
	})
	if !strings.Contains(out, "warn: experimental skill experimental is included in this install") {
		t.Fatalf("expected experimental warning, got: %q", out)
	}
	if _, err := os.Stat(filepath.Join(experimentalCodexDir, "SKILL.md")); err != nil {
		t.Fatalf("experimental skill should be installed with include flag: %v", err)
	}
}

func TestInstallOne_ExperimentalWarnsAndInstalls(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	codexDir := filepath.Join(home, ".codex", "skills", "experimental/")
	writeFixtureSkill(t, skillsRoot, "experimental", filepath.Join(home, ".claude", "commands", "experimental.md"), codexDir)

	reg := &config.Registry{
		Managed:      []string{"experimental"},
		Experimental: []string{"experimental"},
	}
	opts := Options{
		Target:           "codex",
		RunningInstaller: "0.1.0",
		ManifestSchema:   1,
		SkillsRoot:       skillsRoot,
	}

	out := captureStdout(t, func() {
		if err := One("experimental", reg, opts); err != nil {
			t.Fatalf("install one: %v", err)
		}
	})
	if !strings.Contains(out, "warn: experimental skill experimental is included in this install") {
		t.Fatalf("expected experimental warning, got: %q", out)
	}
	if _, err := os.Stat(filepath.Join(codexDir, "SKILL.md")); err != nil {
		t.Fatalf("experimental named install should install: %v", err)
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
	withNonInteractiveStdin(t)
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
	// A sibling .backup file exists.
	entries, err := os.ReadDir(filepath.Dir(claudePath))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range entries {
		if e.Name() == filepath.Base(claudePath)+".backup" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a .backup file alongside %s", claudePath)
	}
}

func TestInstall_Managed_IdenticalExistingFileIsRecorded(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	claudePath := filepath.Join(home, ".claude", "commands", "fixture.md")
	codexDir := filepath.Join(home, ".codex", "skills", "fixture/")
	skillDir := writeFixtureSkill(t, skillsRoot, "fixture", claudePath, codexDir)

	if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(filepath.Join(skillDir, "payload", "claude", filepath.Base(claudePath)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudePath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &config.Registry{Managed: []string{"fixture"}}
	opts := Options{
		Target:           "claude",
		RunningInstaller: "0.1.0",
		ManifestSchema:   1,
		SkillsRoot:       skillsRoot,
	}
	if err := One("fixture", reg, opts); err != nil {
		t.Fatalf("install identical existing file: %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Skills["fixture"].Targets["claude"].Files) != 1 {
		t.Fatalf("expected identical existing file to be recorded, got %+v", s.Skills["fixture"].Targets)
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

func TestInstall_DirectDelegatedPrivateMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"browse": {Repo: "github.com/charlesnpx/browse", Ref: "main", Visibility: "private", Optional: true},
	}}
	err := One("browse", reg, Options{RunningInstaller: "0.1.0", ManifestSchema: 1})
	if err == nil {
		t.Fatal("expected delegated install error")
	}
	if !strings.Contains(err.Error(), "private/team-only") || !strings.Contains(err.Error(), "authentication") {
		t.Fatalf("expected private metadata hint, got: %v", err)
	}
}

func TestInstallAll_SkipsOptionalDelegatedButFailsStrict(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"browse": {Repo: "github.com/charlesnpx/browse", Ref: "main", Visibility: "private", Optional: true},
	}}
	if err := All(reg, Options{RunningInstaller: "0.1.0", ManifestSchema: 1}); err != nil {
		t.Fatalf("optional delegated skill should be skipped by default: %v", err)
	}
	if err := All(reg, Options{RunningInstaller: "0.1.0", ManifestSchema: 1, Strict: true}); err == nil {
		t.Fatal("expected strict install --all to fail on skipped delegated skill")
	}
}

func TestInstall_ExternalToolAlreadyPresentIsRecorded(t *testing.T) {
	home := withFakeHome(t)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	toolPath := writeFakeExecutable(t, bin, "markitdown", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", bin)

	reg := &config.Registry{ExternalTools: map[string]config.ExternalToolSpec{
		"markitdown": {
			Executable: "markitdown",
			Manager:    "pipx",
			Package:    "markitdown[all]",
			Optional:   true,
			RequiredBy: []string{"ado-query"},
		},
	}}
	if err := One("markitdown", reg, Options{Target: "tools"}); err != nil {
		t.Fatalf("install external tool: %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := s.ExternalTools["markitdown"]
	if !ok {
		t.Fatalf("markitdown not recorded: %+v", s.ExternalTools)
	}
	if rec.Path != toolPath || rec.Installed {
		t.Fatalf("bad external tool record: %+v", rec)
	}
}

func TestInstall_ExternalToolMissingUsesPipx(t *testing.T) {
	home := withFakeHome(t)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(home, "pipx.log")
	writeFakeExecutable(t, bin, "pipx", `#!/bin/sh
echo "$@" > "`+logPath+`"
/bin/cat > "`+filepath.Join(bin, "markitdown")+`" <<'EOF'
#!/bin/sh
exit 0
EOF
/bin/chmod 755 "`+filepath.Join(bin, "markitdown")+`"
`)
	t.Setenv("PATH", bin)

	reg := &config.Registry{ExternalTools: map[string]config.ExternalToolSpec{
		"markitdown": {
			Executable: "markitdown",
			Manager:    "pipx",
			Package:    "markitdown[all]",
			Optional:   true,
		},
	}}
	if err := One("markitdown", reg, Options{Target: "tools"}); err != nil {
		t.Fatalf("install external tool: %v", err)
	}
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(body)); got != "install markitdown[all]" {
		t.Fatalf("unexpected pipx call: %q", got)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !s.ExternalTools["markitdown"].Installed {
		t.Fatalf("expected installed=true: %+v", s.ExternalTools["markitdown"])
	}
}

func TestInstall_ExternalToolTargetCodexSkips(t *testing.T) {
	home := withFakeHome(t)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	reg := &config.Registry{ExternalTools: map[string]config.ExternalToolSpec{
		"markitdown": {
			Executable:       "markitdown",
			Manager:          "pipx",
			Package:          "markitdown[all]",
			InstallByDefault: true,
			Optional:         true,
		},
	}}
	if err := All(reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install all target codex: %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.ExternalTools) != 0 {
		t.Fatalf("external tools should not be recorded for codex target: %+v", s.ExternalTools)
	}
}

func TestInstallAll_ExternalToolOptionalFailureWarnsButStrictFails(t *testing.T) {
	home := withFakeHome(t)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	reg := &config.Registry{ExternalTools: map[string]config.ExternalToolSpec{
		"markitdown": {
			Executable:       "markitdown",
			Manager:          "pipx",
			Package:          "markitdown[all]",
			InstallByDefault: true,
			Optional:         true,
		},
	}}
	if err := All(reg, Options{Target: "all"}); err != nil {
		t.Fatalf("optional missing external tool should warn, not fail: %v", err)
	}
	if err := All(reg, Options{Target: "all", Strict: true}); err == nil {
		t.Fatal("expected strict install --all to fail on missing external tool")
	}
}

func TestInstall_ExternalToolDirectMissingPipxFailsClearly(t *testing.T) {
	home := withFakeHome(t)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	reg := &config.Registry{ExternalTools: map[string]config.ExternalToolSpec{
		"markitdown": {
			Executable: "markitdown",
			Manager:    "pipx",
			Package:    "markitdown[all]",
			Optional:   true,
		},
	}}
	err := One("markitdown", reg, Options{Target: "tools"})
	if err == nil || !strings.Contains(err.Error(), "pipx not found") {
		t.Fatalf("expected pipx guidance, got %v", err)
	}
}

func TestDoctorAndList_ExternalTools(t *testing.T) {
	home := withFakeHome(t)
	bin := filepath.Join(home, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFakeExecutable(t, bin, "markitdown", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", bin)

	reg := &config.Registry{ExternalTools: map[string]config.ExternalToolSpec{
		"markitdown": {
			Executable: "markitdown",
			Manager:    "pipx",
			Package:    "markitdown[all]",
			Optional:   true,
			RequiredBy: []string{"ado-query"},
		},
	}}
	var doctorOut bytes.Buffer
	if err := Doctor(&doctorOut, reg, Options{Target: "tools"}); err != nil {
		t.Fatalf("doctor: %v\n%s", err, doctorOut.String())
	}
	if !strings.Contains(doctorOut.String(), "ok: external tool markitdown found") {
		t.Fatalf("doctor output missing external tool ok:\n%s", doctorOut.String())
	}

	s := state.Empty()
	var listOut bytes.Buffer
	PrintList(&listOut, s, reg)
	if !strings.Contains(listOut.String(), "markitdown") || !strings.Contains(listOut.String(), "required by: ado-query") {
		t.Fatalf("list output missing external tool details:\n%s", listOut.String())
	}
}

func writeSetupFixtureSkill(t *testing.T, root, name, envName string, capabilities []string) {
	t.Helper()
	skillDir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(skillDir, "codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "codex", "SKILL.md"), []byte("# setup fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var caps strings.Builder
	for _, capability := range capabilities {
		caps.WriteString("  - ")
		caps.WriteString(capability)
		caps.WriteString("\n")
	}
	manifest := `name: ` + name + `
version: 0.1.0
manifest_schema: 1
min_installer: "0.0.0"
capabilities:
` + caps.String() + `targets:
  codex:
    type: skill_dir
    payload: codex
    install_to: ~/.codex/skills/` + name + `
setup:
  - kind: env
    env: ` + envName + `
    value_class: secret
    required_for:
      - query
`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSetup_ManagedEnvMissing(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	writeSetupFixtureSkill(t, skillsRoot, "fixture", "FIXTURE_TOKEN", []string{"query"})

	reg := &config.Registry{Managed: []string{"fixture"}}
	outcome := EvaluateSetup(reg, Options{Target: "codex", SkillsRoot: skillsRoot}, SetupOptions{})
	if outcome.Kind != SetupIncomplete {
		t.Fatalf("kind = %s, want %s: %+v", outcome.Kind, SetupIncomplete, outcome)
	}
	if len(outcome.Results) != 1 || outcome.Results[0].State != SetupMissing {
		t.Fatalf("results = %+v", outcome.Results)
	}
	if !strings.Contains(outcome.Results[0].Remediation, "FIXTURE_TOKEN") {
		t.Fatalf("missing remediation: %+v", outcome.Results[0])
	}
}

func TestSetup_DedupesSharedRequirements(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	writeSetupFixtureSkill(t, skillsRoot, "alpha", "SHARED_TOKEN", []string{"query"})
	writeSetupFixtureSkill(t, skillsRoot, "beta", "SHARED_TOKEN", []string{"query"})

	reg := &config.Registry{Managed: []string{"alpha", "beta"}}
	outcome := EvaluateSetup(reg, Options{Target: "codex", SkillsRoot: skillsRoot}, SetupOptions{})
	if outcome.Kind != SetupIncomplete {
		t.Fatalf("kind = %s, want %s", outcome.Kind, SetupIncomplete)
	}
	if len(outcome.Results) != 1 {
		t.Fatalf("expected deduped single result, got %+v", outcome.Results)
	}
	if len(outcome.Results[0].Origins) != 2 {
		t.Fatalf("expected both origins, got %+v", outcome.Results[0].Origins)
	}
}

func TestSetup_CapabilityFilterWithNoRequirementsIsSatisfied(t *testing.T) {
	home := withFakeHome(t)
	skillsRoot := filepath.Join(home, "skills")
	skillDir := filepath.Join(skillsRoot, "fixture")
	if err := os.MkdirAll(filepath.Join(skillDir, "codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "codex", "SKILL.md"), []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(`name: fixture
version: 0.1.0
manifest_schema: 1
min_installer: "0.0.0"
capabilities:
  - deploy
targets:
  codex:
    type: skill_dir
    payload: codex
    install_to: ~/.codex/skills/fixture
`), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &config.Registry{Managed: []string{"fixture"}}
	outcome := EvaluateSetup(reg, Options{Target: "codex", SkillsRoot: skillsRoot}, SetupOptions{
		Skill:      "fixture",
		Capability: config.CapabilityDeploy,
	})
	if outcome.Kind != SetupSatisfied || len(outcome.Results) != 0 {
		t.Fatalf("expected satisfied empty plan, got %+v", outcome)
	}
}

func writeDelegatedRepo(t *testing.T, root, name string, behavior string) string {
	t.Helper()
	repo := filepath.Join(root, name+"-repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
set -eu
op=install
target=all
install_root="$HOME"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --plan) op=plan ;;
    --install) op=install ;;
    --uninstall) op=uninstall ;;
    --target) shift; target="$1" ;;
    --install-root) shift; install_root="$1" ;;
    --json) ;;
  esac
  shift
done
	log="$HOME/installer.log"
echo "$op:$target" >> "$log"
file="$install_root/.codex/skills/` + name + `/SKILL.md"
json_name="` + name + `"
case "` + behavior + `" in
  bad-json)
    echo "not json"
    exit 0
    ;;
  outside-root)
    file="$HOME/outside-root/SKILL.md"
    ;;
  wrong-name)
    json_name="wrong"
    ;;
esac
if [ "$op" = "install" ]; then
  mkdir -p "$(dirname "$file")"
  printf '# delegated ` + name + `\n' > "$file"
  if [ "` + behavior + `" = "executable-file" ]; then
    chmod 755 "$file"
  fi
fi
if [ "$op" = "uninstall" ]; then
  rm -f "$file"
fi
sha=""
if [ -f "$file" ]; then
  sha=$(shasum -a 256 "$file" | awk '{print $1}')
fi
if [ "$op" = "install" ]; then
  printf '{"schema":1,"name":"%s","version":"0.2.0","operation":"%s","kind":"delegated","capabilities":["query"],"setup":[{"kind":"env","env":"DELEGATED_TOKEN","value_class":"secret","required_for":["query"]}],"targets":{"codex":{"files":[{"path":"%s","sha256":"%s"}]}},"warnings":[]}\n' "$json_name" "$op" "$file" "$sha"
else
  printf '{"schema":1,"name":"%s","version":"0.2.0","operation":"%s","kind":"delegated","capabilities":["query"],"setup":[{"kind":"env","env":"DELEGATED_TOKEN","value_class":"secret","required_for":["query"]}],"targets":{"codex":{"files":[{"path":"%s"}]}},"warnings":[]}\n' "$json_name" "$op" "$file"
fi
`
	if err := os.WriteFile(filepath.Join(repo, "install-skill.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	run(t, repo, "git", "config", "user.name", "Test")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "init")
	return repo
}

func writeDelegatedInstallerToolRepo(t *testing.T, root, name string) string {
	t.Helper()
	repo := filepath.Join(root, name+"-repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
set -eu
op=install
target=all
install_root="$HOME"
while [ "$#" -gt 0 ]; do
  case "$1" in
    --plan) op=plan ;;
    --install) op=install ;;
    --uninstall) op=uninstall ;;
    --target) shift; target="$1" ;;
    --install-root) shift; install_root="$1" ;;
    --json) ;;
  esac
  shift
done
log="$HOME/installer.log"
echo "$op:$target" >> "$log"
codex_file="$install_root/.codex/skills/` + name + `/SKILL.md"
tool_file="$install_root/.local/bin/` + name + `"
include_codex=0
include_tools=0
case "$target" in
  all|codex)
    include_codex=1
    include_tools=1
    ;;
  tools)
    include_tools=1
    ;;
esac
if [ "$op" = "install" ]; then
  if [ "$include_codex" = "1" ]; then
    mkdir -p "$(dirname "$codex_file")"
    printf '# delegated ` + name + `\n' > "$codex_file"
  fi
  if [ "$include_tools" = "1" ]; then
    mkdir -p "$(dirname "$tool_file")"
    printf '#!/bin/sh\nprintf delegated-tool\n' > "$tool_file"
    chmod 755 "$tool_file"
  fi
fi
if [ "$op" = "uninstall" ]; then
  rm -f "$codex_file" "$tool_file"
fi
codex_sha=""
tool_sha=""
if [ -f "$codex_file" ]; then
  codex_sha=$(shasum -a 256 "$codex_file" | awk '{print $1}')
fi
if [ -f "$tool_file" ]; then
  tool_sha=$(shasum -a 256 "$tool_file" | awk '{print $1}')
fi
printf '{"schema":1,"name":"` + name + `","version":"0.2.0","operation":"%s","kind":"delegated","capabilities":["query"],"setup":[{"kind":"env","env":"DELEGATED_TOKEN","value_class":"secret","required_for":["query"]}],"targets":{' "$op"
sep=""
if [ "$include_codex" = "1" ]; then
  printf '%s"codex":{"files":[{"path":"%s"' "$sep" "$codex_file"
  if [ "$op" = "install" ]; then
    printf ',"sha256":"%s"' "$codex_sha"
  fi
  printf '}]}'
  sep=","
fi
if [ "$include_tools" = "1" ]; then
  printf '%s"tools":{"files":[{"path":"%s"' "$sep" "$tool_file"
  if [ "$op" = "install" ]; then
    printf ',"sha256":"%s"' "$tool_sha"
  fi
  printf '}]}'
fi
printf '},"warnings":[]}\n'
`
	if err := os.WriteFile(filepath.Join(repo, "install-skill.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "git", "init", "-b", "main")
	run(t, repo, "git", "config", "user.email", "test@example.com")
	run(t, repo, "git", "config", "user.name", "Test")
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "init")
	return repo
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

func TestInstall_DelegatedRunsPlanBeforeInstall(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Ref: "main"},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated: %v", err)
	}
	logBody, err := os.ReadFile(filepath.Join(home, "installer.log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(logBody); got != "plan:codex\ninstall:codex\n" {
		t.Fatalf("unexpected installer order:\n%s", got)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	sk := s.Skills["delegated"]
	if sk.Kind != "delegated" || sk.Repo != repo || sk.Ref != "main" || sk.Version != "0.2.0" {
		t.Fatalf("bad delegated state: %+v", sk)
	}
}

func TestInstall_DelegatedPipxToolInstallsFromResolvedCheckout(t *testing.T) {
	home := withFakeHome(t)
	bin := withFakePipx(t, home, "delegated")
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {
			Repo: repo,
			Ref:  "main",
			Tools: []config.DelegatedToolSpec{{
				Executable:  "delegated",
				Manager:     "pipx",
				Package:     "delegated",
				InstallFrom: "checkout",
			}},
		},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated: %v", err)
	}
	logBody, err := os.ReadFile(filepath.Join(home, "pipx.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logBody), "install --force "+filepath.Join(home, ".cache", "mise-en-place", "repos", "delegated")) {
		t.Fatalf("pipx should install from resolved checkout, got:\n%s", logBody)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	sk := s.Skills["delegated"]
	if len(sk.Tools) != 1 || sk.Tools[0].Executable != "delegated" || sk.Tools[0].Path != filepath.Join(bin, "delegated") {
		t.Fatalf("bad delegated tool state: %+v", sk.Tools)
	}
	if len(sk.Targets["codex"].Files) != 1 {
		t.Fatalf("codex target should still be recorded: %+v", sk.Targets)
	}
}

func TestInstall_DelegatedPipxToolTargetToolsSkipsInstaller(t *testing.T) {
	home := withFakeHome(t)
	withFakePipx(t, home, "delegated")
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	pyproject := `[project]
name = "delegated"
version = "0.7.0"
`
	if err := os.WriteFile(filepath.Join(repo, "pyproject.toml"), []byte(pyproject), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "add pyproject")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {
			Repo: repo,
			Ref:  "main",
			Tools: []config.DelegatedToolSpec{{
				Executable: "delegated",
				Manager:    "pipx",
				Package:    "delegated",
			}},
		},
	}}
	if err := One("delegated", reg, Options{Target: "tools"}); err != nil {
		t.Fatalf("install delegated tools: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "installer.log")); !os.IsNotExist(err) {
		t.Fatalf("target tools should not run delegated installer, stat err: %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	sk := s.Skills["delegated"]
	if sk.Version != "0.7.0" {
		t.Fatalf("expected pyproject version for tools-only install, got %+v", sk)
	}
	if len(sk.Targets) != 0 || len(sk.Tools) != 1 {
		t.Fatalf("expected only delegated tool state, got targets=%+v tools=%+v", sk.Targets, sk.Tools)
	}
}

func TestInstall_DelegatedInstallerOwnedToolTargetRunsInstaller(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedInstallerToolRepo(t, home, "delegated")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Ref: "main"},
	}}
	if err := One("delegated", reg, Options{Target: "tools"}); err != nil {
		t.Fatalf("install delegated tools: %v", err)
	}
	logBody, err := os.ReadFile(filepath.Join(home, "installer.log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(logBody); got != "plan:tools\ninstall:tools\n" {
		t.Fatalf("target tools should run delegated installer, got:\n%s", got)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	sk := s.Skills["delegated"]
	if len(sk.Tools) != 0 {
		t.Fatalf("installer-owned tool should not use pipx tool state: %+v", sk.Tools)
	}
	realHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	files := sk.Targets["tools"].Files
	if len(files) != 1 || files[0].Path != filepath.Join(realHome, ".local", "bin", "delegated") {
		t.Fatalf("expected installer-owned tools target, got %+v", sk.Targets)
	}
}

func TestInstall_DelegatedInstallerCanReturnToolsWithCodexTarget(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedInstallerToolRepo(t, home, "delegated")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Ref: "main"},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated codex: %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	sk := s.Skills["delegated"]
	if len(sk.Targets["codex"].Files) != 1 || len(sk.Targets["tools"].Files) != 1 {
		t.Fatalf("expected codex and tools targets, got %+v", sk.Targets)
	}
}

func TestUpgrade_DelegatedInstallerOwnedToolReinstallsWhenFileMissing(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedInstallerToolRepo(t, home, "delegated")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Ref: "main"},
	}}
	if err := One("delegated", reg, Options{Target: "tools"}); err != nil {
		t.Fatalf("install delegated tools: %v", err)
	}
	if err := os.Remove(filepath.Join(home, ".local", "bin", "delegated")); err != nil {
		t.Fatal(err)
	}
	if err := Upgrade("delegated", reg, Options{Target: "tools"}); err != nil {
		t.Fatalf("upgrade delegated tools: %v", err)
	}
	logBody, err := os.ReadFile(filepath.Join(home, "installer.log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(logBody), "install:tools"); got != 2 {
		t.Fatalf("expected upgrade to reinstall missing installer-owned tool, got %d installs:\n%s", got, logBody)
	}
}

func TestDoctor_DelegatedInstallerOwnedToolUsesContractAndState(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedInstallerToolRepo(t, home, "delegated")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Ref: "main"},
	}}
	if err := One("delegated", reg, Options{Target: "tools"}); err != nil {
		t.Fatalf("install delegated tools: %v", err)
	}
	var out bytes.Buffer
	if err := Doctor(&out, reg, Options{Target: "tools"}); err != nil {
		t.Fatalf("doctor delegated tools: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "pipx") {
		t.Fatalf("installer-owned tools should not emit pipx guidance:\n%s", out.String())
	}
	if err := os.Remove(filepath.Join(home, ".local", "bin", "delegated")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	err := Doctor(&out, reg, Options{Target: "tools"})
	if err == nil || !strings.Contains(out.String(), "tools target missing") {
		t.Fatalf("expected missing installer-owned tool error, err=%v out=\n%s", err, out.String())
	}
}

func TestDoctor_DelegatedPipxToolReportsMissingExecutable(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {
			Repo: repo,
			Ref:  "main",
			Tools: []config.DelegatedToolSpec{{
				Executable: "delegated",
				Manager:    "pipx",
				Package:    "delegated",
			}},
		},
	}}
	var out bytes.Buffer
	err := Doctor(&out, reg, Options{Target: "tools"})
	if err == nil || !strings.Contains(out.String(), "missing delegated tool executable delegated") {
		t.Fatalf("expected delegated tool doctor error, err=%v out=\n%s", err, out.String())
	}
}

func TestUpgrade_DelegatedPipxToolReinstallsWhenExecutableMissing(t *testing.T) {
	home := withFakeHome(t)
	bin := withFakePipx(t, home, "delegated")
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {
			Repo: repo,
			Ref:  "main",
			Tools: []config.DelegatedToolSpec{{
				Executable: "delegated",
				Manager:    "pipx",
				Package:    "delegated",
			}},
		},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated: %v", err)
	}
	if err := os.Remove(filepath.Join(bin, "delegated")); err != nil {
		t.Fatal(err)
	}
	if err := Upgrade("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("upgrade delegated: %v", err)
	}
	logBody, err := os.ReadFile(filepath.Join(home, "pipx.log"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(logBody), "install --force"); got != 2 {
		t.Fatalf("expected upgrade to reinstall missing delegated tool, got %d installs:\n%s", got, logBody)
	}
}

func TestUninstall_DelegatedPipxToolCallsPipxUninstall(t *testing.T) {
	home := withFakeHome(t)
	withFakePipx(t, home, "delegated")
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {
			Repo: repo,
			Ref:  "main",
			Tools: []config.DelegatedToolSpec{{
				Executable: "delegated",
				Manager:    "pipx",
				Package:    "delegated",
			}},
		},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated: %v", err)
	}
	if err := Uninstall("delegated"); err != nil {
		t.Fatalf("uninstall delegated: %v", err)
	}
	logBody, err := os.ReadFile(filepath.Join(home, "pipx.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(logBody), "uninstall delegated") {
		t.Fatalf("expected pipx uninstall, got:\n%s", logBody)
	}
}

func TestSetup_DelegatedSetupContract(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Ref: "main"},
	}}
	outcome := EvaluateSetup(reg, Options{Target: "codex"}, SetupOptions{
		Skill:      "delegated",
		Capability: config.CapabilityQuery,
	})
	if outcome.Kind != SetupIncomplete {
		t.Fatalf("kind = %s, want %s: %+v", outcome.Kind, SetupIncomplete, outcome)
	}
	if len(outcome.Results) != 1 || outcome.Results[0].Key != "env:DELEGATED_TOKEN" {
		t.Fatalf("unexpected results: %+v", outcome.Results)
	}
}

func TestInstall_DelegatedLatestReleaseRecordsResolvedRef(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	run(t, repo, "git", "tag", "v0.2.0")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Channel: "latest-release", FallbackRef: "main"},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated: %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	sk := s.Skills["delegated"]
	if sk.Ref != "v0.2.0" || sk.Commit == "" || sk.Channel != "latest-release" || sk.FallbackRef != "main" || sk.ConfiguredRef != "" {
		t.Fatalf("bad delegated channel state: %+v", sk)
	}
}

func TestInstall_DelegatedLatestReleaseUsesFallbackWithoutTags(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Channel: "latest-release", FallbackRef: "main"},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated: %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	sk := s.Skills["delegated"]
	if sk.Ref != "main" || sk.Commit == "" || sk.Channel != "latest-release" || sk.FallbackRef != "main" {
		t.Fatalf("bad fallback delegated state: %+v", sk)
	}
}

func TestInstall_DelegatedLatestReleaseRequiresTagOrFallback(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Channel: "latest-release"},
	}}
	err := One("delegated", reg, Options{Target: "codex"})
	if err == nil || !strings.Contains(err.Error(), "no stable release tags") {
		t.Fatalf("expected missing tag/fallback error, got %v", err)
	}
}

func TestInstall_LatestStableReleaseTagSortsSemver(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	for _, tag := range []string{"v0.9.0", "v0.10.0", "v0.10.0-beta.1", "not-a-version", "v0.2.5"} {
		run(t, repo, "git", "tag", tag)
	}
	got, err := latestStableReleaseTag(repo)
	if err != nil {
		t.Fatalf("latestStableReleaseTag: %v", err)
	}
	if got != "v0.10.0" {
		t.Fatalf("latest tag = %q, want v0.10.0", got)
	}
}

func TestInstall_DelegatedBadJSONAndNameMismatchFail(t *testing.T) {
	home := withFakeHome(t)
	badJSON := writeDelegatedRepo(t, home, "badjson", "bad-json")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"badjson": {Repo: badJSON, Ref: "main"},
	}}
	if err := One("badjson", reg, Options{Target: "codex"}); err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Fatalf("expected JSON error, got %v", err)
	}

	wrongName := writeDelegatedRepo(t, home, "wrongname", "wrong-name")
	reg = &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"wrongname": {Repo: wrongName, Ref: "main"},
	}}
	if err := One("wrongname", reg, Options{Target: "codex"}); err == nil || !strings.Contains(err.Error(), "name mismatch") {
		t.Fatalf("expected name mismatch, got %v", err)
	}
}

func TestInstall_DelegatedCollisionBlocksBeforeMutation(t *testing.T) {
	home := withFakeHome(t)
	withNonInteractiveStdin(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	file := filepath.Join(home, ".codex", "skills", "delegated", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("handmade\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Ref: "main"},
	}}
	err := One("delegated", reg, Options{Target: "codex"})
	if err == nil || !strings.Contains(err.Error(), "unmanaged") {
		t.Fatalf("expected unmanaged collision, got %v", err)
	}
	body, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "handmade\n" {
		t.Fatalf("installer mutated file before collision check: %q", string(body))
	}
}

func TestInstall_DelegatedRejectsStagedPathOutsideInstallRoot(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "outside-root")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Ref: "main"},
	}}
	err := One("delegated", reg, Options{Target: "codex"})
	if err == nil || !strings.Contains(err.Error(), "escapes --install-root") {
		t.Fatalf("expected staged path validation failure, got %v", err)
	}
}

func TestInstall_DelegatedUninstallRemovesStateAndFiles(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Ref: "main"},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated: %v", err)
	}
	if err := Uninstall("delegated"); err != nil {
		t.Fatalf("uninstall delegated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "delegated", "SKILL.md")); err == nil {
		t.Fatal("delegated file should be removed")
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Skills["delegated"]; ok {
		t.Fatal("delegated state should be removed")
	}
}

func TestInstall_DelegatedUpgradeUpdatesState(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Ref: "main"},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated: %v", err)
	}

	scriptPath := filepath.Join(repo, "install-skill.sh")
	body, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(string(body), `"version":"0.2.0"`, `"version":"0.3.0"`))
	if err := os.WriteFile(scriptPath, body, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "upgrade")

	if err := Upgrade("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("upgrade delegated: %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Skills["delegated"].Version; got != "0.3.0" {
		t.Fatalf("expected upgraded version, got %s", got)
	}
}

func TestInstall_DelegatedUpgradeSkipsWhenCurrent(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	run(t, repo, "git", "tag", "v0.2.0")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Channel: "latest-release"},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated: %v", err)
	}
	if err := Upgrade("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("upgrade delegated: %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Skills["delegated"].Version; got != "0.2.0" {
		t.Fatalf("expected current version to remain, got %s", got)
	}
}

func TestInstall_DelegatedUpgradeLatestReleaseUpdatesState(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "ok")
	run(t, repo, "git", "tag", "v0.2.0")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Channel: "latest-release"},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated: %v", err)
	}

	scriptPath := filepath.Join(repo, "install-skill.sh")
	body, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(string(body), `"version":"0.2.0"`, `"version":"0.3.0"`))
	if err := os.WriteFile(scriptPath, body, 0o755); err != nil {
		t.Fatal(err)
	}
	run(t, repo, "git", "add", ".")
	run(t, repo, "git", "commit", "-m", "upgrade")
	run(t, repo, "git", "tag", "v0.3.0")

	if err := Upgrade("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("upgrade delegated: %v", err)
	}
	s, err := state.Load()
	if err != nil {
		t.Fatal(err)
	}
	sk := s.Skills["delegated"]
	if sk.Version != "0.3.0" || sk.Ref != "v0.3.0" || sk.Commit == "" || sk.Channel != "latest-release" {
		t.Fatalf("expected upgraded latest state, got %+v", sk)
	}
}

func TestInstall_DelegatedPreservesExecutableMode(t *testing.T) {
	home := withFakeHome(t)
	repo := writeDelegatedRepo(t, home, "delegated", "executable-file")
	reg := &config.Registry{Delegated: map[string]config.DelegatedRepo{
		"delegated": {Repo: repo, Ref: "main"},
	}}
	if err := One("delegated", reg, Options{Target: "codex"}); err != nil {
		t.Fatalf("install delegated: %v", err)
	}
	path := filepath.Join(home, ".codex", "skills", "delegated", "SKILL.md")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat installed file: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("expected executable mode to be preserved, got %v", info.Mode().Perm())
	}
}
