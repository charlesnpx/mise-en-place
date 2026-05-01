package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/charlesnpx/mise-en-place/internal/artifact"
	"github.com/charlesnpx/mise-en-place/internal/config"
	"github.com/charlesnpx/mise-en-place/internal/state"
)

// Doctor reports registry, manifest, delegated-contract, and installed-file
// drift. It returns an error when non-optional checks fail.
func Doctor(w io.Writer, reg *config.Registry, opts Options) error {
	s, err := state.Load()
	if err != nil {
		return err
	}
	var issues int
	var warnings int

	fmt.Fprintln(w, "mise-en-place doctor")
	fmt.Fprintln(w, "====================")
	fmt.Fprintln(w, "ok: registry loaded")

	for _, name := range reg.Managed {
		if err := doctorManaged(w, s, name, opts); err != nil {
			issues++
			fmt.Fprintf(w, "error: managed %s: %v\n", name, err)
		}
	}

	dnames := make([]string, 0, len(reg.Delegated))
	for name := range reg.Delegated {
		dnames = append(dnames, name)
	}
	sort.Strings(dnames)
	for _, name := range dnames {
		err := doctorDelegated(w, s, name, reg.Delegated[name], opts)
		if err == nil {
			continue
		}
		if reg.Delegated[name].IsOptional() {
			warnings++
			fmt.Fprintf(w, "warn: delegated %s: %v\n", name, err)
			continue
		}
		issues++
		fmt.Fprintf(w, "error: delegated %s: %v\n", name, err)
	}

	for name, sk := range s.Skills {
		if err := doctorInstalled(w, name, sk); err != nil {
			issues++
			fmt.Fprintf(w, "error: installed %s: %v\n", name, err)
		}
	}

	fmt.Fprintf(w, "\nsummary: %d issue(s), %d warning(s)\n", issues, warnings)
	if issues > 0 {
		return fmt.Errorf("doctor found %d issue(s)", issues)
	}
	return nil
}

func doctorManaged(w io.Writer, s *state.State, name string, opts Options) error {
	skillDir := filepath.Join(opts.skillsRoot(), name)
	manifest, err := config.LoadSkillManifest(filepath.Join(skillDir, "skill.yaml"))
	if err != nil {
		return err
	}
	if err := checkMinInstaller(manifest.MinInstaller, opts.RunningInstaller); err != nil {
		return err
	}
	if manifest.ManifestSchema > opts.ManifestSchema {
		return fmt.Errorf("requires manifest_schema=%d but this build understands %d", manifest.ManifestSchema, opts.ManifestSchema)
	}
	plan, err := buildPlan(skillDir, manifest, requestedTargets(manifest, "all"))
	if err != nil {
		return err
	}
	if err := checkCollisions(s, name, plan); err != nil {
		return err
	}
	if err := checkUnmanaged(s, name, plan, false); err != nil {
		return err
	}
	fmt.Fprintf(w, "ok: managed %s manifest and destinations\n", name)
	return nil
}

func doctorDelegated(w io.Writer, s *state.State, name string, repo config.DelegatedRepo, opts Options) error {
	repoDir, err := prepareDelegatedRepo(name, repo)
	if err != nil {
		return err
	}
	installer, err := delegatedInstaller(repoDir)
	if err != nil {
		return err
	}
	target := opts.Target
	if target == "" {
		target = "all"
	}
	planned, err := runDelegatedInstaller(installer, name, "plan", target, false)
	if err != nil {
		return err
	}
	plan, err := delegatedPlan(planned)
	if err != nil {
		return err
	}
	if err := checkCollisions(s, name, plan); err != nil {
		return err
	}
	if err := checkUnmanaged(s, name, plan, false); err != nil {
		return err
	}
	fmt.Fprintf(w, "ok: delegated %s installer plan contract\n", name)
	return nil
}

func doctorInstalled(w io.Writer, name string, sk state.Skill) error {
	for targetName, target := range sk.Targets {
		for _, file := range target.Files {
			if _, err := os.Stat(file.Path); err != nil {
				return fmt.Errorf("%s target missing %s: %w", targetName, file.Path, err)
			}
			sum, err := artifact.SHA256File(file.Path)
			if err != nil {
				return fmt.Errorf("%s target hash %s: %w", targetName, file.Path, err)
			}
			if sum != file.SHA256 {
				return fmt.Errorf("%s target hash drift at %s", targetName, file.Path)
			}
		}
	}
	fmt.Fprintf(w, "ok: installed %s files match state\n", name)
	return nil
}
