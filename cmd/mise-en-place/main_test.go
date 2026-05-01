package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallWithoutSkillDefaultsToAll(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	registryRoot := filepath.Join(home, "registry")
	skillDir := filepath.Join(registryRoot, "skills", "fixture")
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
targets:
  codex:
    type: skill_dir
    payload: codex
    install_to: ~/.codex/skills/fixture
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(registryRoot, "registry.yaml"), []byte("managed:\n  - fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MISE_EN_PLACE_HOME", registryRoot)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"install", "--target", "codex"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install without skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "fixture", "SKILL.md")); err != nil {
		t.Fatalf("expected fixture to be installed via default --all behavior: %v", err)
	}
}
