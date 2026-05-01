package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Registry is the parsed shape of registry.yaml. It declares which skills
// live in this monorepo (managed) and which delegate to external repos.
type Registry struct {
	Managed   []string                 `yaml:"managed"`
	Delegated map[string]DelegatedRepo `yaml:"delegated"`
}

// DelegatedRepo pins a polyrepo skill to a specific tag.
type DelegatedRepo struct {
	Repo       string `yaml:"repo"`
	Ref        string `yaml:"ref"`
	Visibility string `yaml:"visibility"` // public | private; defaults to public
	Optional   bool   `yaml:"optional"`   // optional delegated repos are skipped by install --all unless --strict
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
	if len(r.Managed) == 0 && len(r.Delegated) == 0 {
		return errors.New("registry.yaml: must declare at least one managed or delegated skill")
	}
	for name, d := range r.Delegated {
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

// Kind reports whether a skill is managed, delegated, or unknown to the registry.
func (r *Registry) Kind(name string) string {
	for _, m := range r.Managed {
		if m == name {
			return "managed"
		}
	}
	if _, ok := r.Delegated[name]; ok {
		return "delegated"
	}
	return ""
}
