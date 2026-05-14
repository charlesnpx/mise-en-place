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
	writeTestSkill := func(name string) {
		t.Helper()
		skillDir := filepath.Join(registryRoot, "skills", name)
		if err := os.MkdirAll(filepath.Join(skillDir, "codex"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "codex", "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "skill.yaml"), []byte(`name: `+name+`
version: 0.1.0
manifest_schema: 1
min_installer: "0.0.0"
targets:
  codex:
    type: skill_dir
    payload: codex
    install_to: ~/.codex/skills/`+name+`
`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeTestSkill("fixture")
	writeTestSkill("experimental")
	if err := os.WriteFile(filepath.Join(registryRoot, "registry.yaml"), []byte(`managed:
  - fixture
  - experimental
experimental:
  - experimental
`), 0o644); err != nil {
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
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "experimental", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("expected experimental skill to be skipped by default install, stat err: %v", err)
	}

	cmd = newRootCmd()
	cmd.SetArgs([]string{"install", "--all", "--target", "codex"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install --all: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".codex", "skills", "experimental", "SKILL.md")); err != nil {
		t.Fatalf("expected experimental skill to be installed by explicit --all: %v", err)
	}
}
