package install

import (
	"bytes"
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

func writeFakeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
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
  printf '{"schema":1,"name":"%s","version":"0.2.0","operation":"%s","kind":"delegated","targets":{"codex":{"files":[{"path":"%s","sha256":"%s"}]}},"warnings":[]}\n' "$json_name" "$op" "$file" "$sha"
else
  printf '{"schema":1,"name":"%s","version":"0.2.0","operation":"%s","kind":"delegated","targets":{"codex":{"files":[{"path":"%s"}]}},"warnings":[]}\n' "$json_name" "$op" "$file"
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
