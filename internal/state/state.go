// Package state manages ~/.local/state/mise-en-place/.
//
// state.json is the canonical record of what's installed. It is rewritten
// transactionally via a tempfile + os.Rename, under an advisory flock on
// state.lock. history.jsonl is an append-only audit log written after the
// state commit succeeds.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ManifestSchema is the on-disk schema version for state.json. Bump when
// the State struct changes shape in a way old readers cannot handle.
const ManifestSchema = 1

// State is the in-memory representation of ~/.local/state/mise-en-place/state.json.
type State struct {
	ManifestSchema int                     `json:"manifest_schema"`
	MepVersion     string                  `json:"mise_en_place_version"`
	Skills         map[string]Skill        `json:"skills"`
	ExternalTools  map[string]ExternalTool `json:"external_tools,omitempty"`
}

// Skill is one installed skill's record. Kind is "managed" or "delegated".
type Skill struct {
	Kind           string                  `json:"kind"`
	Version        string                  `json:"version"`
	ManifestSchema int                     `json:"manifest_schema,omitempty"`
	Source         string                  `json:"source,omitempty"` // managed: artifact URL or "local:<path>"
	SHA256         string                  `json:"sha256,omitempty"` // managed: artifact hash
	Repo           string                  `json:"repo,omitempty"`   // delegated: github.com/...
	Ref            string                  `json:"ref,omitempty"`    // delegated: resolved git ref used for install/uninstall
	Commit         string                  `json:"commit,omitempty"` // delegated: resolved commit used for install
	ConfiguredRef  string                  `json:"configured_ref,omitempty"`
	Channel        string                  `json:"channel,omitempty"`
	FallbackRef    string                  `json:"fallback_ref,omitempty"`
	Targets        map[string]TargetRecord `json:"targets,omitempty"`
	Tools          []DelegatedTool         `json:"tools,omitempty"`
	Adopted        bool                    `json:"adopted,omitempty"`
	InstalledAt    time.Time               `json:"installed_at"`
}

// TargetRecord is the per-host installed file list.
type TargetRecord struct {
	Files []FileRecord `json:"files"`
}

// FileRecord is one file owned by a managed skill.
type FileRecord struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// DelegatedTool records a CLI executable installed for a delegated repo by
// mise-en-place.
type DelegatedTool struct {
	Executable    string    `json:"executable"`
	Path          string    `json:"path"`
	Manager       string    `json:"manager"`
	Package       string    `json:"package"`
	InstallSource string    `json:"install_source"`
	Installed     bool      `json:"installed"`
	InstalledAt   time.Time `json:"installed_at,omitempty"`
	VerifiedAt    time.Time `json:"verified_at"`
}

// ExternalTool records a third-party executable verified or installed by
// mise-en-place. These tools are not file-owned by mise-en-place.
type ExternalTool struct {
	Executable  string    `json:"executable"`
	Path        string    `json:"path"`
	Manager     string    `json:"manager"`
	Package     string    `json:"package"`
	Installed   bool      `json:"installed"`
	RequiredBy  []string  `json:"required_by,omitempty"`
	InstalledAt time.Time `json:"installed_at,omitempty"`
	VerifiedAt  time.Time `json:"verified_at"`
}

// Empty returns a freshly-initialised State.
func Empty() *State {
	return &State{
		ManifestSchema: ManifestSchema,
		Skills:         map[string]Skill{},
		ExternalTools:  map[string]ExternalTool{},
	}
}

// StateDir returns ~/.local/state/mise-en-place, creating it if missing.
func StateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".local", "state", "mise-en-place")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// statePath / historyPath / lockPath return the canonical paths.
func statePath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func historyPath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.jsonl"), nil
}

func lockPath() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.lock"), nil
}

// Load reads state.json. Missing file returns an empty State.
func Load() (*State, error) {
	path, err := statePath()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Empty(), nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse state.json: %w", err)
	}
	if s.Skills == nil {
		s.Skills = map[string]Skill{}
	}
	if s.ExternalTools == nil {
		s.ExternalTools = map[string]ExternalTool{}
	}
	if s.ManifestSchema == 0 {
		s.ManifestSchema = ManifestSchema
	}
	return &s, nil
}

// Save writes state.json atomically via tempfile + rename.
//
// The caller must hold the flock for the duration of any read-modify-write
// cycle that includes this call.
func Save(s *State) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if s.ManifestSchema == 0 {
		s.ManifestSchema = ManifestSchema
	}
	if s.Skills == nil {
		s.Skills = map[string]Skill{}
	}
	if s.ExternalTools == nil {
		s.ExternalTools = map[string]ExternalTool{}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".state-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort: if rename succeeds the temp file is gone; if not,
		// remove it so we don't leak.
		_ = os.Remove(tmpName)
	}()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
