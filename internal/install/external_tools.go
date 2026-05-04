package install

import (
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/charlesnpx/mise-en-place/internal/config"
	"github.com/charlesnpx/mise-en-place/internal/state"
)

func externalToolsTargetRequested(target string) bool {
	return target == "" || target == "all" || target == "tools"
}

func installExternalTool(name string, spec config.ExternalToolSpec, opts Options) error {
	if !externalToolsTargetRequested(opts.Target) {
		fmt.Printf("skipping external tool %s for --target=%s\n", name, opts.Target)
		return nil
	}

	path, alreadyPresent := externalToolPath(spec.Executable)
	installed := false
	if !alreadyPresent {
		if spec.Manager != "pipx" {
			return fmt.Errorf("external tool %s uses unsupported manager %q", name, spec.Manager)
		}
		pipx, err := exec.LookPath("pipx")
		if err != nil {
			return fmt.Errorf("pipx not found; install with: brew install pipx && pipx ensurepath")
		}
		cmd := exec.Command(pipx, "install", spec.Package)
		out, err := cmd.CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("pipx install %s failed: %s", spec.Package, msg)
		}
		installed = true
		path, alreadyPresent = externalToolPath(spec.Executable)
		if !alreadyPresent {
			return fmt.Errorf("%s installed with pipx but %s is not on PATH; run: pipx ensurepath", spec.Package, spec.Executable)
		}
	}

	now := time.Now().UTC()
	record := state.ExternalTool{
		Executable: spec.Executable,
		Path:       path,
		Manager:    spec.Manager,
		Package:    spec.Package,
		Installed:  installed,
		RequiredBy: append([]string(nil), spec.RequiredBy...),
		VerifiedAt: now,
	}
	sort.Strings(record.RequiredBy)
	if installed {
		record.InstalledAt = now
	}

	lock, err := state.Lock()
	if err != nil {
		return err
	}
	defer state.Release(lock)

	s, err := state.Load()
	if err != nil {
		return err
	}
	if prior, ok := s.ExternalTools[name]; ok && !installed {
		record.Installed = prior.Installed
		record.InstalledAt = prior.InstalledAt
	}
	s.ExternalTools[name] = record
	if err := state.Save(s); err != nil {
		return err
	}
	_ = state.AppendHistory(state.HistoryEvent{
		Op:      "install",
		Skill:   name,
		Version: spec.Package,
		Targets: []string{"tools"},
		Extra: map[string]string{
			"kind":       "external_tool",
			"executable": spec.Executable,
			"path":       path,
			"manager":    spec.Manager,
		},
	})

	if installed {
		fmt.Printf("installed external tool %s via pipx at %s\n", name, path)
	} else {
		fmt.Printf("external tool %s already available at %s\n", name, path)
	}
	return nil
}

func uninstallExternalTool(name string) error {
	lock, err := state.Lock()
	if err != nil {
		return err
	}
	defer state.Release(lock)

	s, err := state.Load()
	if err != nil {
		return err
	}
	if _, ok := s.ExternalTools[name]; !ok {
		return fmt.Errorf("%w: %s", errNotInstalled, name)
	}
	delete(s.ExternalTools, name)
	if err := state.Save(s); err != nil {
		return err
	}
	_ = state.AppendHistory(state.HistoryEvent{
		Op:    "uninstall",
		Skill: name,
		Extra: map[string]string{"kind": "external_tool"},
	})
	fmt.Printf("forgot external tool %s (executable left installed)\n", name)
	return nil
}

func externalToolPath(executable string) (string, bool) {
	path, err := exec.LookPath(executable)
	if errors.Is(err, exec.ErrNotFound) || err != nil {
		return "", false
	}
	return path, true
}
