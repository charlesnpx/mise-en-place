package install

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charlesnpx/mise-en-place/internal/config"
	"github.com/charlesnpx/mise-en-place/internal/state"
)

const azureDevOpsResource = "499b84ac-1321-427f-aa17-267ca6975798"

type SetupOutcomeKind string

const (
	SetupSatisfied     SetupOutcomeKind = "satisfied"
	SetupIncomplete    SetupOutcomeKind = "incomplete"
	SetupPlanningError SetupOutcomeKind = "planning-error"
	SetupMisuse        SetupOutcomeKind = "misuse"
)

type SetupState string

const (
	SetupOK              SetupState = "ok"
	SetupMissing         SetupState = "missing"
	SetupUnavailable     SetupState = "unavailable"
	SetupUnauthenticated SetupState = "unauthenticated"
	SetupInaccessible    SetupState = "inaccessible"
	SetupInvalid         SetupState = "invalid"
	SetupExpired         SetupState = "expired"
)

type SetupOptions struct {
	Skill         string
	Capability    config.SetupCapability
	InstalledOnly bool
	Check         bool
	JSON          bool
	Interactive   bool
}

type SetupOutcome struct {
	Kind     SetupOutcomeKind `json:"kind"`
	Results  []SetupResult    `json:"results,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
	Errors   []string         `json:"errors,omitempty"`
}

type SetupResult struct {
	Key         string                 `json:"key"`
	Kind        config.SetupKind       `json:"kind"`
	State       SetupState             `json:"state"`
	Detail      string                 `json:"detail,omitempty"`
	ValueClass  config.SetupValueClass `json:"value_class,omitempty"`
	Remediation string                 `json:"remediation,omitempty"`
	Origins     []SetupOrigin          `json:"origins"`
}

type SetupOrigin struct {
	Skill        string                   `json:"skill"`
	Capabilities []config.SetupCapability `json:"capabilities,omitempty"`
}

type setupRegistration struct {
	Name         string
	Capabilities []config.SetupCapability
	Setup        []config.SetupRequirement
	Optional     bool
}

type plannedRequirement struct {
	Key     string
	Req     config.SetupRequirement
	Origins []SetupOrigin
}

type ExitError struct {
	Code    int
	Message string
}

func (e ExitError) Error() string {
	return e.Message
}

func (e ExitError) ExitCode() int {
	return e.Code
}

func RunSetup(w io.Writer, reg *config.Registry, opts Options, setupOpts SetupOptions) error {
	if setupOpts.Interactive && !isInteractiveStdin() {
		outcome := SetupOutcome{Kind: SetupMisuse, Errors: []string{"--interactive requires a terminal"}}
		writeSetupOutput(w, "mise-en-place setup", outcome, setupOpts)
		return ExitError{Code: setupExitCode(outcome.Kind), Message: "setup misuse"}
	}
	outcome := EvaluateSetup(reg, opts, setupOpts)
	writeSetupOutput(w, "mise-en-place setup", outcome, setupOpts)
	if code := setupExitCode(outcome.Kind); code != 0 {
		return ExitError{Code: code, Message: "setup " + string(outcome.Kind)}
	}
	return nil
}

func RunHealth(w io.Writer, reg *config.Registry, opts Options, setupOpts SetupOptions) error {
	setupOpts.InstalledOnly = true
	outcome := EvaluateSetup(reg, opts, setupOpts)
	writeSetupOutput(w, "mise-en-place health", outcome, setupOpts)
	if code := setupExitCode(outcome.Kind); code != 0 {
		return ExitError{Code: code, Message: "health " + string(outcome.Kind)}
	}
	return nil
}

func EvaluateSetup(reg *config.Registry, opts Options, setupOpts SetupOptions) SetupOutcome {
	setupOpts.Skill = reg.CanonicalName(setupOpts.Skill)
	if setupOpts.Capability != "" && !validCapability(setupOpts.Capability) {
		return SetupOutcome{Kind: SetupMisuse, Errors: []string{fmt.Sprintf("unknown capability %q", setupOpts.Capability)}}
	}
	registrations, warnings, errors := setupRegistrations(reg, opts, setupOpts)
	if len(errors) > 0 {
		return SetupOutcome{Kind: SetupPlanningError, Warnings: warnings, Errors: errors}
	}
	if setupOpts.Skill != "" && len(registrations) == 0 {
		return SetupOutcome{Kind: SetupMisuse, Errors: []string{fmt.Sprintf("unknown skill %q", setupOpts.Skill)}}
	}
	if setupOpts.Skill != "" && setupOpts.Capability != "" && len(registrations) == 1 && !registrationHasCapability(registrations[0], setupOpts.Capability) {
		return SetupOutcome{
			Kind:     SetupMisuse,
			Warnings: warnings,
			Errors:   []string{fmt.Sprintf("skill %s does not declare capability %s", setupOpts.Skill, setupOpts.Capability)},
		}
	}

	planned, planErrors := planSetup(registrations, setupOpts)
	if len(planErrors) > 0 {
		return SetupOutcome{Kind: SetupPlanningError, Warnings: warnings, Errors: planErrors}
	}
	results := make([]SetupResult, 0, len(planned))
	for _, item := range planned {
		results = append(results, evaluateRequirement(item))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Key < results[j].Key })
	kind := SetupSatisfied
	for _, result := range results {
		if result.State != SetupOK {
			kind = SetupIncomplete
			break
		}
	}
	return SetupOutcome{Kind: kind, Results: results, Warnings: warnings}
}

func setupRegistrations(reg *config.Registry, opts Options, setupOpts SetupOptions) ([]setupRegistration, []string, []string) {
	installed := map[string]bool{}
	if setupOpts.InstalledOnly {
		s, err := state.Load()
		if err != nil {
			return nil, nil, []string{err.Error()}
		}
		for name := range s.Skills {
			installed[name] = true
			installed[reg.CanonicalName(name)] = true
		}
	}

	var registrations []setupRegistration
	var warnings []string
	var errors []string
	for _, name := range reg.Managed {
		if !setupIncludesSkill(name, installed, setupOpts) {
			continue
		}
		manifest, err := config.LoadSkillManifest(filepath.Join(opts.skillsRoot(), name, "skill.yaml"))
		if err != nil {
			errors = append(errors, fmt.Sprintf("managed %s: %v", name, err))
			continue
		}
		registrations = append(registrations, setupRegistration{
			Name:         name,
			Capabilities: manifest.Capabilities,
			Setup:        manifest.Setup,
		})
	}

	dnames := make([]string, 0, len(reg.Delegated))
	for name := range reg.Delegated {
		dnames = append(dnames, name)
	}
	sort.Strings(dnames)
	for _, name := range dnames {
		repo := reg.Delegated[name]
		if !setupIncludesSkill(name, installed, setupOpts) {
			continue
		}
		checkout, err := prepareDelegatedRepo(name, repo)
		if err != nil {
			msg := fmt.Sprintf("delegated %s: %v", name, err)
			if repo.IsOptional() && setupOpts.Skill == "" {
				warnings = append(warnings, msg)
			} else {
				errors = append(errors, msg)
			}
			continue
		}
		target := opts.Target
		if target == "" {
			target = "all"
		}
		if target == "tools" && len(repo.Tools) > 0 {
			continue
		}
		installer, err := delegatedInstaller(checkout.Dir)
		if err != nil {
			errors = append(errors, fmt.Sprintf("delegated %s: %v", name, err))
			continue
		}
		planned, err := runDelegatedInstaller(installer, name, "plan", target, false, "")
		if err != nil {
			errors = append(errors, fmt.Sprintf("delegated %s: %v", name, err))
			continue
		}
		registrations = append(registrations, setupRegistration{
			Name:         name,
			Capabilities: planned.Capabilities,
			Setup:        planned.Setup,
			Optional:     repo.IsOptional(),
		})
	}
	return registrations, warnings, errors
}

func setupIncludesSkill(name string, installed map[string]bool, setupOpts SetupOptions) bool {
	if setupOpts.Skill != "" && setupOpts.Skill != name {
		return false
	}
	if setupOpts.InstalledOnly && !installed[name] {
		return false
	}
	return true
}

func planSetup(registrations []setupRegistration, setupOpts SetupOptions) ([]plannedRequirement, []string) {
	byKey := map[string]*plannedRequirement{}
	for _, registration := range registrations {
		capabilities := registration.Capabilities
		if len(capabilities) == 0 {
			capabilities = inferCapabilities(registration.Setup)
		}
		if setupOpts.Capability != "" && !containsCapability(capabilities, setupOpts.Capability) {
			continue
		}
		for _, req := range registration.Setup {
			reqCapabilities := req.RequiredFor
			if setupOpts.Capability != "" && len(reqCapabilities) > 0 && !containsCapability(reqCapabilities, setupOpts.Capability) {
				continue
			}
			key := setupKey(req)
			if key == "" {
				return nil, []string{fmt.Sprintf("%s: setup requirement has no canonical key", registration.Name)}
			}
			origin := SetupOrigin{Skill: registration.Name, Capabilities: reqCapabilities}
			if setupOpts.Capability != "" {
				origin.Capabilities = []config.SetupCapability{setupOpts.Capability}
			}
			if existing, ok := byKey[key]; ok {
				merged, err := mergeRequirement(existing.Req, req)
				if err != nil {
					return nil, []string{fmt.Sprintf("%s: %v", key, err)}
				}
				existing.Req = merged
				existing.Origins = append(existing.Origins, origin)
				continue
			}
			byKey[key] = &plannedRequirement{Key: key, Req: req, Origins: []SetupOrigin{origin}}
		}
	}
	out := make([]plannedRequirement, 0, len(byKey))
	for _, item := range byKey {
		item.Origins = uniqueOrigins(item.Origins)
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func evaluateRequirement(item plannedRequirement) SetupResult {
	result := SetupResult{
		Key:         item.Key,
		Kind:        item.Req.Kind,
		ValueClass:  item.Req.ValueClass,
		Remediation: item.Req.Remediation,
		Origins:     item.Origins,
	}
	switch item.Req.Kind {
	case config.SetupEnv:
		value := strings.TrimSpace(os.Getenv(item.Req.Env))
		if value == "" {
			result.State = SetupMissing
			result.Detail = fmt.Sprintf("environment variable %s is not set", item.Req.Env)
			if result.Remediation == "" {
				result.Remediation = fmt.Sprintf("export %s=<value>", item.Req.Env)
			}
			return result
		}
		result.State = SetupOK
		if item.Req.ValueClass == config.ValueSecret {
			result.Detail = fmt.Sprintf("environment variable %s is set", item.Req.Env)
		} else {
			result.Detail = fmt.Sprintf("environment variable %s is set", item.Req.Env)
		}
	case config.SetupExecutable:
		path, ok := externalToolPath(item.Req.Executable)
		if !ok {
			result.State = SetupUnavailable
			result.Detail = fmt.Sprintf("executable %s is not on PATH", item.Req.Executable)
			return result
		}
		result.State = SetupOK
		result.Detail = fmt.Sprintf("executable %s found at %s", item.Req.Executable, path)
	case config.SetupAzureCLIAuth:
		if _, ok := externalToolPath("az"); !ok {
			result.State = SetupUnavailable
			result.Detail = "Azure CLI executable az is not on PATH"
			return result
		}
		if out, err := exec.Command("az", "account", "show", "-o", "none").CombinedOutput(); err != nil {
			result.State = SetupUnauthenticated
			result.Detail = trimCommandOutput(out, "az account show failed; run az login")
			return result
		}
		result.State = SetupOK
		result.Detail = "Azure CLI is authenticated"
	case config.SetupAzureDevOpsToken:
		if _, ok := externalToolPath("az"); !ok {
			result.State = SetupUnavailable
			result.Detail = "Azure CLI executable az is not on PATH"
			return result
		}
		resource := item.Req.Resource
		if resource == "" {
			resource = azureDevOpsResource
		}
		out, err := exec.Command(
			"az", "account", "get-access-token",
			"--resource", resource,
			"--query", "accessToken",
			"-o", "tsv",
		).CombinedOutput()
		if err != nil {
			result.State = SetupUnauthenticated
			result.Detail = trimCommandOutput(out, "could not get Azure DevOps bearer token")
			return result
		}
		if strings.TrimSpace(string(out)) == "" {
			result.State = SetupInvalid
			result.Detail = "Azure CLI returned an empty Azure DevOps bearer token"
			return result
		}
		result.State = SetupOK
		result.Detail = "Azure DevOps bearer token is available from Azure CLI"
	case config.SetupAzureResourceGrp:
		resourceGroup := strings.TrimSpace(os.Getenv(item.Req.Env))
		if resourceGroup == "" {
			result.State = SetupMissing
			result.Detail = fmt.Sprintf("environment variable %s is not set", item.Req.Env)
			if result.Remediation == "" {
				result.Remediation = fmt.Sprintf("export %s=<approved-resource-group>", item.Req.Env)
			}
			return result
		}
		args := []string{"group", "show", "-n", resourceGroup, "-o", "none"}
		if item.Req.SubscriptionEnv != "" {
			subscription := strings.TrimSpace(os.Getenv(item.Req.SubscriptionEnv))
			if subscription == "" {
				result.State = SetupMissing
				result.Detail = fmt.Sprintf("environment variable %s is not set", item.Req.SubscriptionEnv)
				return result
			}
			args = append(args, "--subscription", subscription)
		}
		out, err := exec.Command("az", args...).CombinedOutput()
		if err != nil {
			detail := trimCommandOutput(out, fmt.Sprintf("resource group %s is not accessible", resourceGroup))
			if strings.Contains(detail, "Forbidden") {
				result.State = SetupInaccessible
			} else {
				result.State = SetupInaccessible
			}
			result.Detail = detail
			return result
		}
		result.State = SetupOK
		result.Detail = fmt.Sprintf("resource group %s is accessible", resourceGroup)
	case config.SetupGitHubCLIAuth:
		if _, ok := externalToolPath("gh"); !ok {
			result.State = SetupUnavailable
			result.Detail = "GitHub CLI executable gh is not on PATH"
			return result
		}
		if out, err := exec.Command("gh", "auth", "status").CombinedOutput(); err != nil {
			result.State = SetupUnauthenticated
			result.Detail = trimCommandOutput(out, "gh auth status failed; run gh auth login")
			return result
		}
		result.State = SetupOK
		result.Detail = "GitHub CLI is authenticated"
	default:
		result.State = SetupInvalid
		result.Detail = fmt.Sprintf("unsupported setup kind %s", item.Req.Kind)
	}
	return result
}

func setupKey(req config.SetupRequirement) string {
	switch req.Kind {
	case config.SetupEnv:
		return "env:" + req.Env
	case config.SetupExecutable:
		return "executable:" + req.Executable
	case config.SetupAzureCLIAuth:
		return "azure-cli-auth"
	case config.SetupAzureDevOpsToken:
		return "azure-devops-token:resource=" + req.Resource
	case config.SetupAzureResourceGrp:
		return "azure-resource-group:env=" + req.Env + ";subscription_env=" + req.SubscriptionEnv
	case config.SetupGitHubCLIAuth:
		return "github-cli-auth"
	default:
		return ""
	}
}

func mergeRequirement(a, b config.SetupRequirement) (config.SetupRequirement, error) {
	if a.Kind != b.Kind {
		return config.SetupRequirement{}, fmt.Errorf("cannot merge different setup kinds %s and %s", a.Kind, b.Kind)
	}
	out := a
	if out.ValueClass == "" {
		out.ValueClass = b.ValueClass
	}
	if b.ValueClass == config.ValueSecret {
		out.ValueClass = config.ValueSecret
	}
	if out.Remediation == "" {
		out.Remediation = b.Remediation
	}
	return out, nil
}

func writeSetupOutput(w io.Writer, title string, outcome SetupOutcome, setupOpts SetupOptions) {
	if setupOpts.JSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(outcome)
		return
	}
	if setupOpts.Check {
		fmt.Fprintf(w, "%s: %s\n", title, outcome.Kind)
		return
	}
	fmt.Fprintln(w, title)
	fmt.Fprintln(w, strings.Repeat("=", len(title)))
	for _, warning := range outcome.Warnings {
		fmt.Fprintf(w, "warn: %s\n", warning)
	}
	for _, err := range outcome.Errors {
		fmt.Fprintf(w, "error: %s\n", err)
	}
	if len(outcome.Results) == 0 && len(outcome.Errors) == 0 {
		fmt.Fprintln(w, "ok: no setup requirements")
	}
	for _, result := range outcome.Results {
		prefix := "ok"
		if result.State != SetupOK {
			prefix = string(result.State)
		}
		fmt.Fprintf(w, "%s: %s: %s", prefix, result.Key, result.Detail)
		if by := setupOriginsText(result.Origins); by != "" {
			fmt.Fprintf(w, " (required by: %s)", by)
		}
		fmt.Fprintln(w)
		if result.State != SetupOK && result.Remediation != "" {
			fmt.Fprintf(w, "  remediation: %s\n", result.Remediation)
		}
	}
	ok, incomplete := 0, 0
	for _, result := range outcome.Results {
		if result.State == SetupOK {
			ok++
		} else {
			incomplete++
		}
	}
	fmt.Fprintf(w, "\nsummary: %s (%d ok, %d incomplete, %d warning(s), %d error(s))\n", outcome.Kind, ok, incomplete, len(outcome.Warnings), len(outcome.Errors))
}

func setupExitCode(kind SetupOutcomeKind) int {
	switch kind {
	case SetupSatisfied:
		return 0
	case SetupIncomplete:
		return 1
	case SetupPlanningError:
		return 2
	case SetupMisuse:
		return 3
	default:
		return 2
	}
}

func validCapability(capability config.SetupCapability) bool {
	switch capability {
	case config.CapabilityRead, config.CapabilityQuery, config.CapabilityWrite, config.CapabilityDeploy:
		return true
	default:
		return false
	}
}

func registrationHasCapability(registration setupRegistration, capability config.SetupCapability) bool {
	capabilities := registration.Capabilities
	if len(capabilities) == 0 {
		capabilities = inferCapabilities(registration.Setup)
	}
	return containsCapability(capabilities, capability)
}

func inferCapabilities(reqs []config.SetupRequirement) []config.SetupCapability {
	seen := map[config.SetupCapability]bool{}
	for _, req := range reqs {
		for _, capability := range req.RequiredFor {
			if validCapability(capability) {
				seen[capability] = true
			}
		}
	}
	out := make([]config.SetupCapability, 0, len(seen))
	for capability := range seen {
		out = append(out, capability)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func containsCapability(capabilities []config.SetupCapability, want config.SetupCapability) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}

func uniqueOrigins(origins []SetupOrigin) []SetupOrigin {
	seen := map[string]SetupOrigin{}
	for _, origin := range origins {
		caps := append([]config.SetupCapability(nil), origin.Capabilities...)
		sort.Slice(caps, func(i, j int) bool { return caps[i] < caps[j] })
		origin.Capabilities = caps
		seen[origin.Skill+"|"+capabilitiesText(caps)] = origin
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]SetupOrigin, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func setupOriginsText(origins []SetupOrigin) string {
	parts := make([]string, 0, len(origins))
	for _, origin := range origins {
		if len(origin.Capabilities) == 0 {
			parts = append(parts, origin.Skill)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s/%s", origin.Skill, capabilitiesText(origin.Capabilities)))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func capabilitiesText(capabilities []config.SetupCapability) string {
	parts := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		parts = append(parts, string(capability))
	}
	sort.Strings(parts)
	return strings.Join(parts, "+")
}

func trimCommandOutput(out []byte, fallback string) string {
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return fallback
	}
	return msg
}
