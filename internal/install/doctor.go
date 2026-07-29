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
	"github.com/charlesnpx/mise-en-place/internal/hostpaths"
	"github.com/charlesnpx/mise-en-place/internal/state"
)

const (
	visibleHostClaude = "claude"
	visibleHostCodex  = "codex"
)

type doctorCounts struct {
	issues   int
	warnings int
}

type delegatedDoctorContract struct {
	Checkout delegatedCheckout
	Planned  *delegatedResult
	Target   string
	ToolOnly bool
}

type delegatedVisibility struct {
	Names []string
	Hosts map[string]bool
}

func (v delegatedVisibility) visible() bool {
	return len(v.Names) > 0
}

// Doctor reports registry, manifest, delegated-contract, and installed-file
// drift. It returns an error when non-optional checks fail.
func Doctor(w io.Writer, reg *config.Registry, opts Options) error {
	s, err := state.Load()
	if err != nil {
		return err
	}
	visible, err := inventoryVisibleSkills()
	if err != nil {
		return err
	}
	counts := doctorCounts{}
	var setupRegistrations []setupRegistration

	fmt.Fprintln(w, "mise-en-place doctor")
	fmt.Fprintln(w, "====================")
	fmt.Fprintln(w, "ok: registry loaded")

	for _, name := range reg.Managed {
		manifest, err := doctorManaged(w, s, name, opts)
		if err != nil {
			counts.issues++
			fmt.Fprintf(w, "error: managed %s: %v\n", name, err)
			continue
		}
		if installedAs(s, reg, name) {
			setupRegistrations = append(setupRegistrations, setupRegistration{
				Name:         name,
				Capabilities: manifest.Capabilities,
				Setup:        manifest.Setup,
			})
		}
	}

	dnames := make([]string, 0, len(reg.Delegated))
	for name := range reg.Delegated {
		dnames = append(dnames, name)
	}
	sort.Strings(dnames)
	for _, name := range dnames {
		repo := reg.Delegated[name]
		recorded := installedAs(s, reg, name)
		visibility := visible.match(reg.ProvidedSkills(name))
		orphan := visibility.visible() && !recorded
		applicable := recorded || visibility.visible()

		if shouldSkipBroadDelegated(repo, recorded, opts) && !visibility.visible() {
			fmt.Fprintf(w, "skip: delegated %s: %s\n", name, broadDelegatedSkipReason(repo, recorded, false))
			continue
		}
		if orphan {
			counts.warnings++
			fmt.Fprintf(w, "warn: delegated %s: %s; repair with: mise-en-place install %s\n", name, absentDelegatedStateText(visibility.Names), name)
		}

		contract, err := resolveDelegatedDoctorContract(name, repo, opts)
		if err != nil {
			counts.reportDelegatedContract(w, name, repo, orphan, err)
			continue
		}
		if applicable && contract.Planned != nil {
			setupRegistrations = append(setupRegistrations, setupRegistration{
				Name:         name,
				Capabilities: contract.Planned.Capabilities,
				Setup:        contract.Planned.Setup,
				Optional:     repo.IsOptional(),
			})
		}
		if !orphan {
			if err := validateDelegatedDoctorDestinations(s, name, contract); err != nil {
				counts.reportDelegatedContract(w, name, repo, false, err)
				continue
			}
		}

		if applicable && contract.Planned != nil {
			for _, finding := range delegatedResourceFindings(name, contract, visibility) {
				counts.reportDelegatedDependency(w, name, repo, recorded, orphan, finding)
			}
		}
		for _, finding := range delegatedToolFindings(w, name, repo, contract.Target) {
			counts.reportDelegatedDependency(w, name, repo, recorded, orphan, finding)
		}
		printDelegatedDoctorContractOK(w, name, contract)
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
				counts.warnings++
				fmt.Fprintf(w, "warn: external tool %s: %v\n", name, err)
				continue
			}
			counts.issues++
			fmt.Fprintf(w, "error: external tool %s: %v\n", name, err)
		}
	}

	for name, sk := range s.Skills {
		if err := doctorInstalled(w, name, sk); err != nil {
			counts.issues++
			fmt.Fprintf(w, "error: installed %s: %v\n", name, err)
		}
	}

	setupOutcome := evaluateDoctorSetup(setupRegistrations)
	for _, warning := range setupOutcome.Warnings {
		counts.warnings++
		fmt.Fprintf(w, "warn: setup: %s\n", warning)
	}
	for _, err := range setupOutcome.Errors {
		counts.issues++
		fmt.Fprintf(w, "error: setup: %s\n", err)
	}
	setupWarned := false
	for _, result := range setupOutcome.Results {
		if result.State == SetupOK {
			continue
		}
		counts.warnings++
		setupWarned = true
		fmt.Fprintf(w, "warn: setup %s: %s", result.Key, result.Detail)
		if by := setupOriginsText(result.Origins); by != "" {
			fmt.Fprintf(w, " (required by: %s)", by)
		}
		if result.Remediation != "" {
			fmt.Fprintf(w, "; remediation: %s", result.Remediation)
		}
		fmt.Fprintln(w)
	}
	if len(setupOutcome.Results) > 0 && !setupWarned && len(setupOutcome.Errors) == 0 {
		fmt.Fprintln(w, "ok: applicable skill setup requirements")
	}

	fmt.Fprintf(w, "\nsummary: %d issue(s), %d warning(s)\n", counts.issues, counts.warnings)
	if counts.issues > 0 {
		return fmt.Errorf("doctor found %d issue(s)", counts.issues)
	}
	return nil
}

func (c *doctorCounts) reportDelegatedContract(w io.Writer, name string, repo config.DelegatedRepo, orphan bool, err error) {
	if orphan || repo.IsOptional() {
		c.warnings++
		fmt.Fprintf(w, "warn: delegated %s: %v\n", name, err)
		return
	}
	c.issues++
	fmt.Fprintf(w, "error: delegated %s: %v\n", name, err)
}

func (c *doctorCounts) reportDelegatedDependency(w io.Writer, name string, repo config.DelegatedRepo, recorded, orphan bool, finding string) {
	if orphan || (!recorded && repo.IsOptional()) {
		c.warnings++
		fmt.Fprintf(w, "warn: delegated %s: %s\n", name, finding)
		return
	}
	c.issues++
	fmt.Fprintf(w, "error: delegated %s: %s\n", name, finding)
}

func absentDelegatedStateText(names []string) string {
	if len(names) == 1 {
		return fmt.Sprintf("state is absent for visible skill or command %s", names[0])
	}
	return fmt.Sprintf("state is absent for visible skills or commands %s", strings.Join(names, ", "))
}

func resolveDelegatedDoctorContract(name string, repo config.DelegatedRepo, opts Options) (*delegatedDoctorContract, error) {
	checkout, err := prepareDelegatedRepo(name, repo)
	if err != nil {
		return nil, err
	}
	target := opts.Target
	if target == "" {
		target = "all"
	}
	contract := &delegatedDoctorContract{Checkout: checkout, Target: target}
	if target == "tools" && len(repo.Tools) > 0 {
		contract.ToolOnly = true
		return contract, nil
	}
	installer, err := delegatedInstaller(checkout.Dir)
	if err != nil {
		return nil, err
	}
	planned, err := runDelegatedInstaller(installer, name, "plan", target, false, "")
	if err != nil {
		return nil, err
	}
	contract.Planned = planned
	return contract, nil
}

func validateDelegatedDoctorDestinations(s *state.State, name string, contract *delegatedDoctorContract) error {
	if contract.Planned == nil {
		return nil
	}
	plan, err := delegatedPlan(contract.Planned)
	if err != nil {
		return err
	}
	if err := checkCollisions(s, name, plan); err != nil {
		return err
	}
	return checkDestinationConflicts(s, name, plan)
}

func delegatedResourceFindings(name string, contract *delegatedDoctorContract, visibility delegatedVisibility) []string {
	var findings []string
	repair := fmt.Sprintf("repair with: mise-en-place install %s", name)
	for _, host := range []string{visibleHostClaude, visibleHostCodex} {
		if !visibility.Hosts[host] || !doctorTargetIncludesHost(contract.Target, host) {
			continue
		}
		target, ok := contract.Planned.Targets[host]
		if !ok {
			findings = append(findings, fmt.Sprintf("delegated contract has no %s target for a visible %s payload; %s", host, host, repair))
			continue
		}
		findings = append(findings, missingDelegatedTargetFiles(host, target, repair)...)
	}
	if target, ok := contract.Planned.Targets["tools"]; ok && delegatedPipxToolsRequested(contract.Target) {
		findings = append(findings, missingDelegatedTargetFiles("tools", target, repair)...)
	}
	return findings
}

func doctorTargetIncludesHost(target, host string) bool {
	return target == "" || target == "all" || target == host
}

func missingDelegatedTargetFiles(targetName string, target delegatedTargetResult, repair string) []string {
	var findings []string
	for _, file := range target.Files {
		info, err := os.Stat(file.Path)
		if err != nil {
			if os.IsNotExist(err) {
				findings = append(findings, fmt.Sprintf("%s target missing %s; %s", targetName, file.Path, repair))
			} else {
				findings = append(findings, fmt.Sprintf("%s target cannot access %s: %v; %s", targetName, file.Path, err, repair))
			}
			continue
		}
		if info.IsDir() {
			findings = append(findings, fmt.Sprintf("%s target expected a file at %s; %s", targetName, file.Path, repair))
			continue
		}
	}
	return findings
}

func delegatedToolFindings(w io.Writer, name string, repo config.DelegatedRepo, target string) []string {
	if !delegatedPipxToolsRequested(target) {
		return nil
	}
	var findings []string
	for _, tool := range repo.Tools {
		path, ok := externalToolPath(tool.Executable)
		if !ok {
			findings = append(findings, fmt.Sprintf("missing delegated tool executable %s on PATH; repair with: mise-en-place install %s", tool.Executable, name))
			continue
		}
		fmt.Fprintf(w, "ok: delegated %s tool %s found at %s\n", name, tool.Executable, path)
	}
	return findings
}

func printDelegatedDoctorContractOK(w io.Writer, name string, contract *delegatedDoctorContract) {
	if contract.ToolOnly {
		fmt.Fprintf(w, "ok: delegated %s tool contract\n", name)
		return
	}
	if contract.Checkout.FallbackUsed {
		fmt.Fprintf(w, "ok: delegated %s installer plan contract (fallback %s)\n", name, contract.Checkout.ResolvedRef)
		return
	}
	fmt.Fprintf(w, "ok: delegated %s installer plan contract\n", name)
}

func evaluateDoctorSetup(registrations []setupRegistration) SetupOutcome {
	planned, errors := planSetup(registrations, SetupOptions{})
	if len(errors) > 0 {
		return SetupOutcome{Kind: SetupPlanningError, Errors: errors}
	}
	results := make([]SetupResult, 0, len(planned))
	kind := SetupSatisfied
	for _, item := range planned {
		result := evaluateRequirement(item)
		results = append(results, result)
		if result.State != SetupOK {
			kind = SetupIncomplete
		}
	}
	return SetupOutcome{Kind: kind, Results: results}
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

func doctorManaged(w io.Writer, s *state.State, name string, opts Options) (*config.SkillManifest, error) {
	skillDir := filepath.Join(opts.skillsRoot(), name)
	manifest, err := config.LoadSkillManifest(filepath.Join(skillDir, "skill.yaml"))
	if err != nil {
		return nil, err
	}
	if err := checkMinInstaller(manifest.MinInstaller, opts.RunningInstaller); err != nil {
		return nil, err
	}
	if manifest.ManifestSchema > opts.ManifestSchema {
		return nil, fmt.Errorf("requires manifest_schema=%d but this build understands %d", manifest.ManifestSchema, opts.ManifestSchema)
	}
	plan, err := buildPlan(skillDir, manifest, requestedTargets(manifest, "all"))
	if err != nil {
		return nil, err
	}
	if err := checkCollisions(s, name, plan); err != nil {
		return nil, err
	}
	if err := checkDestinationConflicts(s, name, plan); err != nil {
		return nil, err
	}
	fmt.Fprintf(w, "ok: managed %s manifest and destinations\n", name)
	return manifest, nil
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

type visibleSkillInventory map[string]map[string]bool

func inventoryVisibleSkills() (visibleSkillInventory, error) {
	visible := visibleSkillInventory{}
	for _, source := range []struct {
		root string
		host string
	}{
		{root: hostpaths.CodexSkillsDir(), host: visibleHostCodex},
		{root: hostpaths.ClaudeSkillsDir(), host: visibleHostClaude},
	} {
		if err := visible.addSkillDirectories(source.root, source.host); err != nil {
			return nil, err
		}
	}
	if err := visible.addClaudeCommands(hostpaths.ClaudeCommandsDir()); err != nil {
		return nil, err
	}
	return visible, nil
}

func (v visibleSkillInventory) add(name, host string) {
	if v[name] == nil {
		v[name] = map[string]bool{}
	}
	v[name][host] = true
}

func (v visibleSkillInventory) addSkillDirectories(root, host string) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inventory visible %s skills in %s: %w", host, root, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		entryInfo, err := os.Stat(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inventory visible %s skill entry %s: %w", host, name, err)
		}
		if !entryInfo.IsDir() {
			continue
		}
		info, err := os.Stat(filepath.Join(root, name, "SKILL.md"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inventory visible %s skill %s: %w", host, name, err)
		}
		if !info.IsDir() {
			v.add(name, host)
		}
	}
	return nil
}

func (v visibleSkillInventory) addClaudeCommands(root string) error {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inventory visible Claude commands in %s: %w", root, err)
	}
	for _, entry := range entries {
		filename := entry.Name()
		if strings.HasPrefix(filename, ".") || filepath.Ext(filename) != ".md" {
			continue
		}
		info, err := os.Stat(filepath.Join(root, filename))
		if err != nil {
			return fmt.Errorf("inventory visible Claude command %s: %w", filename, err)
		}
		if info.IsDir() {
			continue
		}
		v.add(strings.TrimSuffix(filename, ".md"), visibleHostClaude)
	}
	return nil
}

func (v visibleSkillInventory) match(provided []string) delegatedVisibility {
	match := delegatedVisibility{Hosts: map[string]bool{}}
	for _, name := range provided {
		hosts := v[name]
		if len(hosts) == 0 {
			continue
		}
		match.Names = append(match.Names, name)
		for host := range hosts {
			match.Hosts[host] = true
		}
	}
	sort.Strings(match.Names)
	return match
}
