// Package config defines the on-disk schemas for skill.yaml and registry.yaml.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TargetType is the install shape for a single host.
//
// command: a single file, e.g. ~/.claude/commands/humanizer.md
// skill_dir: a directory tree, e.g. ~/.codex/skills/humanizer/
type TargetType string

const (
	TargetCommand  TargetType = "command"
	TargetSkillDir TargetType = "skill_dir"
)

// Target describes a single host's install for one skill.
type Target struct {
	Type      TargetType `yaml:"type"`
	Payload   string     `yaml:"payload"`    // path inside the skill dir
	InstallTo string     `yaml:"install_to"` // destination on disk; ~ is expanded
}

// Helper is a vendored support file or directory installed alongside the skill.
type Helper struct {
	Source    string `yaml:"source"`
	InstallTo string `yaml:"install_to"`
}

// SkillManifest is the on-disk shape of skills/<skill>/skill.yaml.
type SkillManifest struct {
	Name           string             `yaml:"name"`
	Version        string             `yaml:"version"`
	ManifestSchema int                `yaml:"manifest_schema"`
	MinInstaller   string             `yaml:"min_installer"`
	Targets        map[string]*Target `yaml:"targets"`
	Helpers        []Helper           `yaml:"helpers,omitempty"`
}

// LoadSkillManifest reads a skill.yaml from disk.
func LoadSkillManifest(path string) (*SkillManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m SkillManifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &m, nil
}

// Validate checks that required fields are present and well-formed.
func (m *SkillManifest) Validate() error {
	if m.Name == "" {
		return errors.New("skill.yaml: name is required")
	}
	if m.Version == "" {
		return errors.New("skill.yaml: version is required")
	}
	if m.ManifestSchema == 0 {
		return errors.New("skill.yaml: manifest_schema is required")
	}
	if m.MinInstaller == "" {
		return errors.New("skill.yaml: min_installer is required")
	}
	if len(m.Targets) == 0 {
		return errors.New("skill.yaml: at least one target must be declared")
	}
	for name, t := range m.Targets {
		if t == nil {
			return fmt.Errorf("skill.yaml: target %s is empty", name)
		}
		if t.Type != TargetCommand && t.Type != TargetSkillDir {
			return fmt.Errorf("skill.yaml: target %s has invalid type %q", name, t.Type)
		}
		if t.Payload == "" {
			return fmt.Errorf("skill.yaml: target %s missing payload", name)
		}
		if t.InstallTo == "" {
			return fmt.Errorf("skill.yaml: target %s missing install_to", name)
		}
	}
	return nil
}

// ResolvedPayload returns the absolute path of the payload for a target,
// rooted at the skill's directory.
func (m *SkillManifest) ResolvedPayload(skillDir, targetName string) (string, error) {
	t, ok := m.Targets[targetName]
	if !ok {
		return "", fmt.Errorf("target %q not declared in skill.yaml", targetName)
	}
	return filepath.Join(skillDir, t.Payload), nil
}
