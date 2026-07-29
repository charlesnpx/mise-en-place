package config

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Registry is the parsed shape of registry.yaml. It declares managed skills,
// delegated repos, and external tools used by those skills.
type Registry struct {
	Managed       []string                    `yaml:"managed"`
	Renames       map[string]string           `yaml:"renames"`
	Delegated     map[string]DelegatedRepo    `yaml:"delegated"`
	Experimental  []string                    `yaml:"experimental"`
	ExternalTools map[string]ExternalToolSpec `yaml:"external_tools"`
}

// DelegatedRepo locates a polyrepo skill. Use Ref for exact pins, or Channel
// for moving sources such as latest-release.
type DelegatedRepo struct {
	Repo        string              `yaml:"repo"`
	Ref         string              `yaml:"ref"`
	Channel     string              `yaml:"channel"`      // latest-release
	FallbackRef string              `yaml:"fallback_ref"` // used by latest-release when no release tags exist
	Visibility  string              `yaml:"visibility"`   // public | private; defaults to public
	Optional    bool                `yaml:"optional"`     // optional delegated repos are skipped by install --all unless --strict
	Provides    []string            `yaml:"provides"`     // visible skill/command names; defaults to the registry key
	Tools       []DelegatedToolSpec `yaml:"tools"`
}

// DelegatedToolSpec declares a CLI executable that belongs to a delegated repo
// and should be installed from the resolved delegated checkout.
type DelegatedToolSpec struct {
	Executable  string `yaml:"executable"`
	Manager     string `yaml:"manager"`      // currently only pipx
	Package     string `yaml:"package"`      // pipx package name, used for uninstall
	InstallFrom string `yaml:"install_from"` // checkout; defaults to checkout
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
		if d.Ref == "" && d.Channel == "" {
			return fmt.Errorf("registry.yaml: delegated %s missing ref or channel", name)
		}
		if d.Ref != "" && d.Channel != "" {
			return fmt.Errorf("registry.yaml: delegated %s must not set both ref and channel", name)
		}
		if d.Channel != "" && d.Channel != "latest-release" {
			return fmt.Errorf("registry.yaml: delegated %s has invalid channel %q (expected latest-release)", name, d.Channel)
		}
		if d.Visibility != "" && d.Visibility != "public" && d.Visibility != "private" {
			return fmt.Errorf("registry.yaml: delegated %s has invalid visibility %q (expected public or private)", name, d.Visibility)
		}
		providedSeen := map[string]bool{}
		for i, provided := range d.Provides {
			label := fmt.Sprintf("registry.yaml: delegated %s provides[%d]", name, i)
			if strings.TrimSpace(provided) == "" {
				return fmt.Errorf("%s is empty", label)
			}
			if provided != strings.TrimSpace(provided) {
				return fmt.Errorf("%s has surrounding whitespace", label)
			}
			if providedSeen[provided] {
				return fmt.Errorf("registry.yaml: delegated %s provides %s more than once", name, provided)
			}
			providedSeen[provided] = true
		}
		for i, tool := range d.Tools {
			label := fmt.Sprintf("registry.yaml: delegated %s tools[%d]", name, i)
			if tool.Executable == "" {
				return fmt.Errorf("%s missing executable", label)
			}
			if tool.Manager == "" {
				return fmt.Errorf("%s missing manager", label)
			}
			if tool.Manager != "pipx" {
				return fmt.Errorf("%s has invalid manager %q (expected pipx)", label, tool.Manager)
			}
			if tool.Package == "" {
				return fmt.Errorf("%s missing package", label)
			}
			if tool.InstallFrom != "" && tool.InstallFrom != "checkout" {
				return fmt.Errorf("%s has invalid install_from %q (expected checkout)", label, tool.InstallFrom)
			}
		}
	}
	providedOwners := map[string]string{}
	for name := range r.Delegated {
		for _, provided := range r.ProvidedSkills(name) {
			if owner := seen[provided]; owner != "" && !(provided == name && owner == "delegated") {
				return fmt.Errorf("registry.yaml: delegated %s provides %s, which is already declared as %s", name, provided, owner)
			}
			if _, ok := r.ExternalTools[provided]; ok {
				return fmt.Errorf("registry.yaml: delegated %s provides %s, which is already declared as an external tool", name, provided)
			}
			if owner := providedOwners[provided]; owner != "" && owner != name {
				return fmt.Errorf("registry.yaml: delegated %s and %s both provide %s", owner, name, provided)
			}
			providedOwners[provided] = name
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
	for oldName, newName := range r.Renames {
		if oldName == "" {
			return errors.New("registry.yaml: rename has empty source name")
		}
		if newName == "" {
			return fmt.Errorf("registry.yaml: rename %s has empty target name", oldName)
		}
		if oldName == newName {
			return fmt.Errorf("registry.yaml: rename %s points to itself", oldName)
		}
		if owner := seen[oldName]; owner != "" {
			return fmt.Errorf("registry.yaml: rename source %s is still declared as %s", oldName, owner)
		}
		owner := seen[newName]
		if owner == "" {
			return fmt.Errorf("registry.yaml: rename %s targets unknown skill %s", oldName, newName)
		}
		if owner == "external tool" {
			return fmt.Errorf("registry.yaml: rename %s targets external tool %s", oldName, newName)
		}
		if _, chained := r.Renames[newName]; chained {
			return fmt.Errorf("registry.yaml: rename %s targets renamed skill %s; renames must point directly at the canonical skill", oldName, newName)
		}
		if owner := providedOwners[oldName]; owner != "" {
			return fmt.Errorf("registry.yaml: rename source %s is also provided by delegated %s", oldName, owner)
		}
	}
	experimentalSeen := map[string]bool{}
	for _, name := range r.Experimental {
		if experimentalSeen[name] {
			return fmt.Errorf("registry.yaml: experimental %s declared more than once", name)
		}
		experimentalSeen[name] = true
		owner := seen[name]
		if owner == "" {
			return fmt.Errorf("registry.yaml: experimental %s is not a managed or delegated skill", name)
		}
		if owner == "external tool" {
			return fmt.Errorf("registry.yaml: experimental %s must be a managed or delegated skill, not an external tool", name)
		}
	}
	return nil
}

// ProvidedSkills returns the visible skill and command names owned by a
// delegated registry entry. Entries without an explicit provides list own a
// skill with the same name as the registry key.
func (r *Registry) ProvidedSkills(name string) []string {
	if r == nil {
		return nil
	}
	repo, ok := r.Delegated[name]
	if !ok {
		return nil
	}
	if len(repo.Provides) == 0 {
		return []string{name}
	}
	out := append([]string(nil), repo.Provides...)
	sort.Strings(out)
	return out
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
	name = r.CanonicalName(name)
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

// IsExperimental reports whether a managed or delegated skill is excluded from
// the default install set and requires explicit opt-in.
func (r *Registry) IsExperimental(name string) bool {
	name = r.CanonicalName(name)
	for _, experimental := range r.Experimental {
		if experimental == name {
			return true
		}
	}
	return false
}

// CanonicalName returns the current registry name for a possibly-renamed skill.
func (r *Registry) CanonicalName(name string) string {
	if r == nil || name == "" {
		return name
	}
	seen := map[string]bool{}
	for {
		next := r.Renames[name]
		if next == "" || seen[name] {
			return name
		}
		seen[name] = true
		name = next
	}
}

// RenameSources returns old names that now resolve to canonicalName.
func (r *Registry) RenameSources(canonicalName string) []string {
	if r == nil {
		return nil
	}
	var out []string
	for oldName, newName := range r.Renames {
		if r.CanonicalName(newName) == canonicalName {
			out = append(out, oldName)
		}
	}
	sort.Strings(out)
	return out
}
