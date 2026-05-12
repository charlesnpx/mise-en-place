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

// SetupKind is a vetted setup check kind. Skills declare facts that
// mise-en-place can verify; they do not provide arbitrary shell setup logic.
type SetupKind string

const (
	SetupEnv              SetupKind = "env"
	SetupExecutable       SetupKind = "executable"
	SetupAzureCLIAuth     SetupKind = "azure-cli-auth"
	SetupAzureDevOpsToken SetupKind = "azure-devops-token"
	SetupAzureResourceGrp SetupKind = "azure-resource-group"
	SetupGitHubCLIAuth    SetupKind = "github-cli-auth"
)

// SetupCapability describes what a requirement enables.
type SetupCapability string

const (
	CapabilityRead   SetupCapability = "read"
	CapabilityQuery  SetupCapability = "query"
	CapabilityWrite  SetupCapability = "write"
	CapabilityDeploy SetupCapability = "deploy"
)

// SetupValueClass controls how setup values are displayed.
type SetupValueClass string

const (
	ValuePlain  SetupValueClass = "plain"
	ValueSecret SetupValueClass = "secret"
)

// SetupRequirement declares one setup prerequisite for a skill.
type SetupRequirement struct {
	Kind            SetupKind         `yaml:"kind" json:"kind"`
	RequiredFor     []SetupCapability `yaml:"required_for,omitempty" json:"required_for,omitempty"`
	Env             string            `yaml:"env,omitempty" json:"env,omitempty"`
	Executable      string            `yaml:"executable,omitempty" json:"executable,omitempty"`
	Resource        string            `yaml:"resource,omitempty" json:"resource,omitempty"`
	SubscriptionEnv string            `yaml:"subscription_env,omitempty" json:"subscription_env,omitempty"`
	ValueClass      SetupValueClass   `yaml:"value_class,omitempty" json:"value_class,omitempty"`
	Remediation     string            `yaml:"remediation,omitempty" json:"remediation,omitempty"`
}

// SkillManifest is the on-disk shape of skills/<skill>/skill.yaml.
type SkillManifest struct {
	Name           string             `yaml:"name"`
	Version        string             `yaml:"version"`
	ManifestSchema int                `yaml:"manifest_schema"`
	MinInstaller   string             `yaml:"min_installer"`
	Capabilities   []SetupCapability  `yaml:"capabilities,omitempty"`
	Targets        map[string]*Target `yaml:"targets"`
	Helpers        []Helper           `yaml:"helpers,omitempty"`
	Setup          []SetupRequirement `yaml:"setup,omitempty"`
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
	if err := ValidateCapabilities(m.Capabilities, "skill.yaml"); err != nil {
		return err
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
	return ValidateSetupRequirements(m.Setup, m.Capabilities, "skill.yaml")
}

// ValidateSetupRequirements checks setup declarations shared by managed
// manifests and delegated installer JSON.
func ValidateSetupRequirements(reqs []SetupRequirement, capabilities []SetupCapability, owner string) error {
	declared := capabilitySet(capabilities)
	for i, req := range reqs {
		if err := req.Validate(); err != nil {
			return fmt.Errorf("%s: setup[%d]: %w", owner, i, err)
		}
		if len(declared) == 0 {
			continue
		}
		for _, capability := range req.RequiredFor {
			if !declared[capability] {
				return fmt.Errorf("%s: setup[%d]: required_for %q is not declared in capabilities", owner, i, capability)
			}
		}
	}
	return nil
}

// ValidateCapabilities checks a skill capability list.
func ValidateCapabilities(capabilities []SetupCapability, owner string) error {
	seen := map[SetupCapability]bool{}
	for i, capability := range capabilities {
		switch capability {
		case CapabilityRead, CapabilityQuery, CapabilityWrite, CapabilityDeploy:
		default:
			return fmt.Errorf("%s: capabilities[%d]: invalid capability %q", owner, i, capability)
		}
		if seen[capability] {
			return fmt.Errorf("%s: duplicate capability %q", owner, capability)
		}
		seen[capability] = true
	}
	return nil
}

func capabilitySet(capabilities []SetupCapability) map[SetupCapability]bool {
	out := map[SetupCapability]bool{}
	for _, capability := range capabilities {
		out[capability] = true
	}
	return out
}

// Validate checks a single setup requirement.
func (r SetupRequirement) Validate() error {
	if r.Kind == "" {
		return errors.New("kind is required")
	}
	for _, capability := range r.RequiredFor {
		switch capability {
		case CapabilityRead, CapabilityQuery, CapabilityWrite, CapabilityDeploy:
		default:
			return fmt.Errorf("invalid required_for capability %q", capability)
		}
	}
	if r.ValueClass != "" {
		switch r.ValueClass {
		case ValuePlain, ValueSecret:
		default:
			return fmt.Errorf("invalid value_class %q", r.ValueClass)
		}
	}
	switch r.Kind {
	case SetupEnv:
		if r.Env == "" {
			return errors.New("env requirement missing env")
		}
	case SetupExecutable:
		if r.Executable == "" {
			return errors.New("executable requirement missing executable")
		}
	case SetupAzureCLIAuth:
		// No additional fields required.
	case SetupAzureDevOpsToken:
		if r.Resource == "" {
			return errors.New("azure-devops-token requirement missing resource")
		}
	case SetupAzureResourceGrp:
		if r.Env == "" {
			return errors.New("azure-resource-group requirement missing env")
		}
	case SetupGitHubCLIAuth:
		// No additional fields required.
	default:
		return fmt.Errorf("invalid kind %q", r.Kind)
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
