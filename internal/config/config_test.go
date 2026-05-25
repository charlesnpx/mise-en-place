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
capabilities:
  - read
targets:
  claude:
    type: command
    payload: payload/claude/humanizer.md
    install_to: ~/.claude/commands/humanizer.md
  codex:
    type: skill_dir
    payload: payload/codex/
    install_to: ~/.codex/skills/humanizer/
setup:
  - kind: env
    env: FIGMA_TOKEN
    value_class: secret
    required_for:
      - read
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
	if len(m.Capabilities) != 1 || m.Capabilities[0] != CapabilityRead {
		t.Errorf("capabilities: %+v", m.Capabilities)
	}
	if len(m.Setup) != 1 || m.Setup[0].Env != "FIGMA_TOKEN" || m.Setup[0].ValueClass != ValueSecret {
		t.Errorf("setup: %+v", m.Setup)
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

func TestLoadSkillManifest_InvalidSetup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skill.yaml")
	if err := os.WriteFile(path, []byte(`
name: bad
version: 0.1.0
manifest_schema: 1
min_installer: "0.1.0"
capabilities:
  - read
targets:
  codex:
    type: skill_dir
    payload: codex
    install_to: ~/.codex/skills/bad
setup:
  - kind: env
    env: TOKEN
    required_for:
      - deploy
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSkillManifest(path); err == nil {
		t.Fatal("expected required_for outside declared capabilities to fail validation")
	}
}

func TestLoadRegistry_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.yaml")
	if err := os.WriteFile(path, []byte(`
managed:
  - humanizer
  - pr
renames:
  review-pr: pr
delegated:
  keyframe:
    repo: github.com/charlesnpx/keyframe
    channel: latest-release
    fallback_ref: main
    visibility: public
    optional: false
    tools:
      - executable: keyframe
        manager: pipx
        package: keyframe
        install_from: checkout
  browse:
    repo: github.com/charlesnpx/browse
    ref: main
    visibility: private
    optional: true
experimental:
  - browse
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
	if r.Kind("review-pr") != "managed" || r.CanonicalName("review-pr") != "pr" {
		t.Errorf("review-pr rename not resolved: kind=%s canonical=%s", r.Kind("review-pr"), r.CanonicalName("review-pr"))
	}
	if got := r.RenameSources("pr"); len(got) != 1 || got[0] != "review-pr" {
		t.Errorf("rename sources: %v", got)
	}
	if r.Kind("keyframe") != "delegated" {
		t.Errorf("keyframe kind: %s", r.Kind("keyframe"))
	}
	if got := r.Delegated["keyframe"].Channel; got != "latest-release" {
		t.Errorf("keyframe channel: %q", got)
	}
	if len(r.Delegated["keyframe"].Tools) != 1 || r.Delegated["keyframe"].Tools[0].Executable != "keyframe" {
		t.Errorf("keyframe tools: %+v", r.Delegated["keyframe"].Tools)
	}
	if r.Kind("markitdown") != "external_tool" {
		t.Errorf("markitdown kind: %s", r.Kind("markitdown"))
	}
	if !r.Delegated["browse"].IsPrivate() || !r.Delegated["browse"].IsOptional() {
		t.Errorf("browse should parse as private optional: %+v", r.Delegated["browse"])
	}
	if !r.IsExperimental("browse") || r.IsExperimental("keyframe") {
		t.Errorf("experimental flags parsed incorrectly: %+v", r.Experimental)
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
		"unknown experimental": `
managed:
  - tool
experimental:
  - other
`,
		"external tool experimental": `
external_tools:
  markitdown:
    executable: markitdown
    manager: pipx
    package: markitdown
experimental:
  - markitdown
`,
		"duplicate experimental": `
managed:
  - tool
experimental:
  - tool
  - tool
`,
		"rename source still declared": `
managed:
  - pr
  - review-pr
renames:
  review-pr: pr
`,
		"rename unknown target": `
managed:
  - pr
renames:
  review-pr: missing
`,
		"rename external tool target": `
external_tools:
  markitdown:
    executable: markitdown
    manager: pipx
    package: markitdown
renames:
  old-markitdown: markitdown
`,
		"rename chain": `
managed:
  - pr
renames:
  old-review-pr: review-pr
  review-pr: pr
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

func TestLoadRegistry_InvalidDelegatedTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.yaml")
	if err := os.WriteFile(path, []byte(`
delegated:
  keyframe:
    repo: github.com/charlesnpx/keyframe
    ref: main
    tools:
      - executable: keyframe
        manager: pip
        package: keyframe
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRegistry(path); err == nil {
		t.Fatal("expected invalid delegated tool manager to fail validation")
	}
}
