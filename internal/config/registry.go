package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Registry is the parsed shape of registry.yaml. It declares managed skills,
// delegated repos, and external tools used by those skills.
type Registry struct {
	Managed       []string                    `yaml:"managed"`
	Delegated     map[string]DelegatedRepo    `yaml:"delegated"`
	ExternalTools map[string]ExternalToolSpec `yaml:"external_tools"`
}

// DelegatedRepo pins a polyrepo skill to a specific tag.
type DelegatedRepo struct {
	Repo       string `yaml:"repo"`
	Ref        string `yaml:"ref"`
	Visibility string `yaml:"visibility"` // public | private; defaults to public
	Optional   bool   `yaml:"optional"`   // optional delegated repos are skipped by install --all unless --strict
}

// ExternalToolSpec declares a third-party executable used by one or more
// skills. mise-en-place verifies the executable and may install it through the
// configured manager.
type ExternalToolSpec struct {
	Executable       string   `yaml:"executable"`
	Manager          string   `yaml:"manager"` // currently only pipx
	Package          string   `yaml:"package"`
	InstallByDefault bool     `yaml:"install_by_default"`
	Optional         bool     `yaml:"optional"`
	RequiredBy       []string `yaml:"required_by"`
}

// IsPrivate reports whether the registry explicitly marks the delegated repo
// private/team-only.
func (d DelegatedRepo) IsPrivate() bool {
	return d.Visibility == "private"
}

// IsOptional reports whether install --all may skip this delegated repo without
// failing the whole run. Private delegated repos should almost always be optional
// in a public registry.
func (d DelegatedRepo) IsOptional() bool {
	return d.Optional
}

// LoadRegistry reads registry.yaml from disk.
func LoadRegistry(path string) (*Registry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Registry
	if err := yaml.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &r, nil
}

// Validate ensures required fields are present.
func (r *Registry) Validate() error {
	if len(r.Managed) == 0 && len(r.Delegated) == 0 && len(r.ExternalTools) == 0 {
		return errors.New("registry.yaml: must declare at least one managed skill, delegated skill, or external tool")
	}
	seen := map[string]string{}
	for _, name := range r.Managed {
		if owner := seen[name]; owner != "" {
			return fmt.Errorf("registry.yaml: %s declared as both %s and managed", name, owner)
		}
		seen[name] = "managed"
	}
	for name, d := range r.Delegated {
		if owner := seen[name]; owner != "" {
			return fmt.Errorf("registry.yaml: %s declared as both %s and delegated", name, owner)
		}
		seen[name] = "delegated"
		if d.Repo == "" {
			return fmt.Errorf("registry.yaml: delegated %s missing repo", name)
		}
		if d.Ref == "" {
			return fmt.Errorf("registry.yaml: delegated %s missing ref", name)
		}
		if d.Visibility != "" && d.Visibility != "public" && d.Visibility != "private" {
			return fmt.Errorf("registry.yaml: delegated %s has invalid visibility %q (expected public or private)", name, d.Visibility)
		}
	}
	for name, t := range r.ExternalTools {
		if owner := seen[name]; owner != "" {
			return fmt.Errorf("registry.yaml: %s declared as both %s and external tool", name, owner)
		}
		seen[name] = "external tool"
		if t.Executable == "" {
			return fmt.Errorf("registry.yaml: external tool %s missing executable", name)
		}
		if t.Manager == "" {
			return fmt.Errorf("registry.yaml: external tool %s missing manager", name)
		}
		if t.Manager != "pipx" {
			return fmt.Errorf("registry.yaml: external tool %s has invalid manager %q (expected pipx)", name, t.Manager)
		}
		if t.Package == "" {
			return fmt.Errorf("registry.yaml: external tool %s missing package", name)
		}
	}
	return nil
}

// AllSkills returns every skill name (managed + delegated) in stable order.
func (r *Registry) AllSkills() []string {
	out := make([]string, 0, len(r.Managed)+len(r.Delegated))
	out = append(out, r.Managed...)
	for name := range r.Delegated {
		out = append(out, name)
	}
	return out
}

// Kind reports whether a registry entry is managed, delegated, external_tool,
// or unknown.
func (r *Registry) Kind(name string) string {
	for _, m := range r.Managed {
		if m == name {
			return "managed"
		}
	}
	if _, ok := r.Delegated[name]; ok {
		return "delegated"
	}
	if _, ok := r.ExternalTools[name]; ok {
		return "external_tool"
	}
	return ""
}
