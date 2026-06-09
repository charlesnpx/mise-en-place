package install

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
		repo := reg.Delegated[name]
		installed := false
		if sk, ok := s.Skills[name]; ok && sk.Kind == "delegated" {
			installed = true
		}
		if shouldSkipBroadDelegated(repo, installed, opts) {
			fmt.Fprintf(w, "skip: delegated %s: %s\n", name, broadDelegatedSkipReason(repo, installed))
			continue
		}
		err := doctorDelegated(w, s, name, repo, opts)
		if err == nil {
			continue
		}
		if repo.IsOptional() {
			warnings++
			fmt.Fprintf(w, "warn: delegated %s: %v\n", name, err)
			continue
		}
		issues++
		fmt.Fprintf(w, "error: delegated %s: %v\n", name, err)
	}

	if externalToolsTargetRequested(opts.Target) {
		tnames := make([]string, 0, len(reg.ExternalTools))
		for name := range reg.ExternalTools {
			tnames = append(tnames, name)
		}
		sort.Strings(tnames)
		for _, name := range tnames {
			err := doctorExternalTool(w, name, reg.ExternalTools[name])
			if err == nil {
				continue
			}
			if reg.ExternalTools[name].Optional {
				warnings++
				fmt.Fprintf(w, "warn: external tool %s: %v\n", name, err)
				continue
			}
			issues++
			fmt.Fprintf(w, "error: external tool %s: %v\n", name, err)
		}
	}

	for name, sk := range s.Skills {
		if err := doctorInstalled(w, name, sk); err != nil {
			issues++
			fmt.Fprintf(w, "error: installed %s: %v\n", name, err)
		}
	}

	setupOutcome := EvaluateSetup(reg, opts, SetupOptions{InstalledOnly: true})
	for _, warning := range setupOutcome.Warnings {
		warnings++
		fmt.Fprintf(w, "warn: setup: %s\n", warning)
	}
	for _, err := range setupOutcome.Errors {
		issues++
		fmt.Fprintf(w, "error: setup: %s\n", err)
	}
	setupWarned := false
	for _, result := range setupOutcome.Results {
		if result.State == SetupOK {
			continue
		}
		warnings++
		setupWarned = true
		fmt.Fprintf(w, "warn: setup %s: %s", result.Key, result.Detail)
		if by := setupOriginsText(result.Origins); by != "" {
			fmt.Fprintf(w, " (required by: %s)", by)
		}
		fmt.Fprintln(w)
	}
	if len(setupOutcome.Results) > 0 && !setupWarned && len(setupOutcome.Errors) == 0 {
		fmt.Fprintln(w, "ok: installed skill setup requirements")
	}

	fmt.Fprintf(w, "\nsummary: %d issue(s), %d warning(s)\n", issues, warnings)
	if issues > 0 {
		return fmt.Errorf("doctor found %d issue(s)", issues)
	}
	return nil
}

func doctorExternalTool(w io.Writer, name string, spec config.ExternalToolSpec) error {
	path, ok := externalToolPath(spec.Executable)
	if !ok {
		msg := fmt.Sprintf("missing executable %s", spec.Executable)
		if spec.Manager == "pipx" {
			msg += fmt.Sprintf("; install with: pipx install %s", spec.Package)
		}
		if len(spec.RequiredBy) > 0 {
			neededBy := append([]string(nil), spec.RequiredBy...)
			sort.Strings(neededBy)
			msg += fmt.Sprintf(" (required by: %s)", strings.Join(neededBy, ", "))
		}
		return fmt.Errorf("%s", msg)
	}
	fmt.Fprintf(w, "ok: external tool %s found at %s\n", name, path)
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
	if err := checkDestinationConflicts(s, name, plan); err != nil {
		return err
	}
	fmt.Fprintf(w, "ok: managed %s manifest and destinations\n", name)
	return nil
}

func doctorDelegated(w io.Writer, s *state.State, name string, repo config.DelegatedRepo, opts Options) error {
	checkout, err := prepareDelegatedRepo(name, repo)
	if err != nil {
		return err
	}
	target := opts.Target
	if target == "" {
		target = "all"
	}
	if !(target == "tools" && len(repo.Tools) > 0) {
		installer, err := delegatedInstaller(checkout.Dir)
		if err != nil {
			return err
		}
		planned, err := runDelegatedInstaller(installer, name, "plan", target, false, "")
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
		if err := checkDestinationConflicts(s, name, plan); err != nil {
			return err
		}
	}
	if delegatedPipxToolsRequested(target) {
		for _, tool := range repo.Tools {
			path, ok := externalToolPath(tool.Executable)
			if !ok {
				return fmt.Errorf("missing delegated tool executable %s; install with: pipx install --force %s", tool.Executable, checkout.Dir)
			}
			fmt.Fprintf(w, "ok: delegated %s tool %s found at %s\n", name, tool.Executable, path)
		}
	}
	if target == "tools" && len(repo.Tools) > 0 {
		fmt.Fprintf(w, "ok: delegated %s tool contract\n", name)
		return nil
	}
	if checkout.FallbackUsed {
		fmt.Fprintf(w, "ok: delegated %s installer plan contract (fallback %s)\n", name, checkout.ResolvedRef)
		return nil
	}
	fmt.Fprintf(w, "ok: delegated %s installer plan contract\n", name)
	return nil
}

func doctorInstalled(w io.Writer, name string, sk state.Skill) error {
	for _, tool := range sk.Tools {
		path, ok := externalToolPath(tool.Executable)
		if !ok {
			return fmt.Errorf("delegated tool missing %s (was %s)", tool.Executable, tool.Path)
		}
		if tool.Path != "" && path != tool.Path {
			return fmt.Errorf("delegated tool path changed for %s: %s -> %s", tool.Executable, tool.Path, path)
		}
	}
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
	fmt.Fprintf(w, "ok: installed %s files and tools match state\n", name)
	return nil
}
