package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkillManifest_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.yaml")
	if err := os.WriteFile(path, []byte(`
name: humanizer
version: 2.3.0
manifest_schema: 1
min_installer: "0.1.0"
targets:
  claude:
    type: command
    payload: payload/claude/humanizer.md
    install_to: ~/.claude/commands/humanizer.md
  codex:
    type: skill_dir
    payload: payload/codex/
    install_to: ~/.codex/skills/humanizer/
`), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadSkillManifest(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "humanizer" {
		t.Errorf("name: got %q", m.Name)
	}
	if len(m.Targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(m.Targets))
	}
	if m.Targets["claude"].Type != TargetCommand {
		t.Errorf("claude target type: %q", m.Targets["claude"].Type)
	}
	if m.Targets["codex"].Type != TargetSkillDir {
		t.Errorf("codex target type: %q", m.Targets["codex"].Type)
	}
}

func TestLoadSkillManifest_MissingFields(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"no name":          "version: 1.0.0\nmanifest_schema: 1\nmin_installer: \"0.1.0\"\ntargets:\n  claude:\n    type: command\n    payload: a\n    install_to: b\n",
		"no min_installer": "name: x\nversion: 1.0.0\nmanifest_schema: 1\ntargets:\n  claude:\n    type: command\n    payload: a\n    install_to: b\n",
		"no targets":       "name: x\nversion: 1.0.0\nmanifest_schema: 1\nmin_installer: \"0.1.0\"\n",
		"bad target type":  "name: x\nversion: 1.0.0\nmanifest_schema: 1\nmin_installer: \"0.1.0\"\ntargets:\n  claude:\n    type: bogus\n    payload: a\n    install_to: b\n",
	}
	for label, body := range cases {
		t.Run(label, func(t *testing.T) {
			path := filepath.Join(dir, label+".yaml")
			_ = os.WriteFile(path, []byte(body), 0o644)
			if _, err := LoadSkillManifest(path); err == nil {
				t.Fatalf("expected error for %s, got nil", label)
			}
		})
	}
}

func TestLoadRegistry_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.yaml")
	if err := os.WriteFile(path, []byte(`
managed:
  - humanizer
  - review-pr
delegated:
  keyframe:
    repo: github.com/charlesnpx/keyframe
    channel: latest-release
    fallback_ref: main
    visibility: public
    optional: false
  browse:
    repo: github.com/charlesnpx/browse
    ref: main
    visibility: private
    optional: true
external_tools:
  markitdown:
    executable: markitdown
    manager: pipx
    package: "markitdown[all]"
    install_by_default: true
    optional: true
    required_by:
      - ado-query
`), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Managed) != 2 {
		t.Errorf("managed: got %v", r.Managed)
	}
	if r.Kind("humanizer") != "managed" {
		t.Errorf("humanizer kind: %s", r.Kind("humanizer"))
	}
	if r.Kind("keyframe") != "delegated" {
		t.Errorf("keyframe kind: %s", r.Kind("keyframe"))
	}
	if got := r.Delegated["keyframe"].Channel; got != "latest-release" {
		t.Errorf("keyframe channel: %q", got)
	}
	if r.Kind("markitdown") != "external_tool" {
		t.Errorf("markitdown kind: %s", r.Kind("markitdown"))
	}
	if !r.Delegated["browse"].IsPrivate() || !r.Delegated["browse"].IsOptional() {
		t.Errorf("browse should parse as private optional: %+v", r.Delegated["browse"])
	}
	if got := r.ExternalTools["markitdown"].Package; got != "markitdown[all]" {
		t.Errorf("markitdown package: %q", got)
	}
	if r.Kind("nope") != "" {
		t.Errorf("unknown skill should return empty: %q", r.Kind("nope"))
	}
}

func TestLoadRegistry_InvalidDelegatedSource(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"missing source": `
delegated:
  tool:
    repo: github.com/charlesnpx/tool
`,
		"ref and channel": `
delegated:
  tool:
    repo: github.com/charlesnpx/tool
    ref: v1.0.0
    channel: latest-release
`,
		"bad channel": `
delegated:
  tool:
    repo: github.com/charlesnpx/tool
    channel: nightly
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".yaml")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadRegistry(path); err == nil {
				t.Fatal("expected delegated source validation error")
			}
		})
	}
}

func TestLoadRegistry_InvalidDelegatedVisibility(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.yaml")
	if err := os.WriteFile(path, []byte(`
delegated:
  browse:
    repo: github.com/charlesnpx/browse
    ref: main
    visibility: team-only
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistry(path); err == nil {
		t.Fatal("expected invalid visibility to fail validation")
	}
}

func TestLoadRegistry_InvalidExternalToolManager(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.yaml")
	if err := os.WriteFile(path, []byte(`
external_tools:
  markitdown:
    executable: markitdown
    manager: pip
    package: "markitdown[all]"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistry(path); err == nil {
		t.Fatal("expected invalid external tool manager to fail validation")
	}
}
