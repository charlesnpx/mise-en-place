package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charlesnpx/mise-en-place/internal/config"
	"github.com/charlesnpx/mise-en-place/internal/state"
)

type delegatedResult struct {
	Schema       int                              `json:"schema"`
	Name         string                           `json:"name"`
	Version      string                           `json:"version"`
	Operation    string                           `json:"operation"`
	Kind         string                           `json:"kind"`
	Capabilities []config.SetupCapability         `json:"capabilities,omitempty"`
	Setup        []config.SetupRequirement        `json:"setup,omitempty"`
	Targets      map[string]delegatedTargetResult `json:"targets"`
	Warnings     []string                         `json:"warnings"`
}

type delegatedTargetResult struct {
	Files []delegatedFileResult `json:"files"`
}

type delegatedFileResult struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

type delegatedCheckout struct {
	Dir           string
	ResolvedRef   string
	Commit        string
	ConfiguredRef string
	Channel       string
	FallbackRef   string
	FallbackUsed  bool
}

func installDelegated(name string, repo config.DelegatedRepo, opts Options) error {
	target := opts.Target
	if target == "" {
		target = "all"
	}
	checkout, err := prepareDelegatedRepo(name, repo)
	if err != nil {
		if repo.IsPrivate() {
			return fmt.Errorf("%w; %s is marked private/team-only, check repository access or authentication", err, name)
		}
		return err
	}
	if checkout.FallbackUsed {
		fmt.Fprintf(os.Stderr, "warn: %s: no stable release tags found; using fallback ref %s\n", name, checkout.ResolvedRef)
	}
	installer, err := delegatedInstaller(checkout.Dir)
	if err != nil {
		return err
	}
	if _, err := runDelegatedInstaller(installer, name, "plan", target, false, ""); err != nil {
		return err
	}
	stageRoot, err := os.MkdirTemp("", "mise-en-place-delegated-*")
	if err != nil {
		return err
	}
	if stageRoot, err = filepath.EvalSymlinks(stageRoot); err != nil {
		return err
	}
	defer os.RemoveAll(stageRoot)

	staged, err := runDelegatedInstaller(installer, name, "install", target, true, stageRoot)
	if err != nil {
		return err
	}
	plan, err := delegatedStagedPlan(staged, stageRoot)
	if err != nil {
		return err
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
	if err := checkCollisions(s, name, plan); err != nil {
		return err
	}
	applied, skipped, err := applyOwnershipPlan(s, name, plan, opts.Backup)
	if err != nil {
		return err
	}

	files, err := recordFiles(applied)
	if err != nil {
		return err
	}

	s.Skills[name] = state.Skill{
		Kind:          "delegated",
		Version:       staged.Version,
		Repo:          repo.Repo,
		Ref:           checkout.ResolvedRef,
		Commit:        checkout.Commit,
		ConfiguredRef: checkout.ConfiguredRef,
		Channel:       checkout.Channel,
		FallbackRef:   checkout.FallbackRef,
		Targets:       files,
		InstalledAt:   time.Now().UTC(),
	}
	if err := state.Save(s); err != nil {
		return err
	}
	_ = state.AppendHistory(state.HistoryEvent{
		Op:      "install",
		Skill:   name,
		Version: staged.Version,
		Targets: delegatedTargetNames(staged.Targets),
	})
	for _, warning := range staged.Warnings {
		fmt.Fprintf(os.Stderr, "warn: %s: %s\n", name, warning)
	}
	fmt.Printf("installed %s %s (%s delegated)\n", name, staged.Version, strings.Join(delegatedTargetNames(staged.Targets), ", "))
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "partial install: skipped %d divergent file(s); skipped files were not recorded in state\n", skipped)
	}
	return nil
}

func upgradeDelegated(name string, repo config.DelegatedRepo, opts Options) error {
	target := opts.Target
	if target == "" {
		target = "all"
	}
	checkout, err := prepareDelegatedRepo(name, repo)
	if err != nil {
		if repo.IsPrivate() {
			return fmt.Errorf("%w; %s is marked private/team-only, check repository access or authentication", err, name)
		}
		return err
	}
	if checkout.FallbackUsed {
		fmt.Fprintf(os.Stderr, "warn: %s: no stable release tags found; using fallback ref %s\n", name, checkout.ResolvedRef)
	}
	installer, err := delegatedInstaller(checkout.Dir)
	if err != nil {
		return err
	}
	planned, err := runDelegatedInstaller(installer, name, "plan", target, false, "")
	if err != nil {
		return err
	}
	s, err := state.Load()
	if err != nil {
		return err
	}
	if current, ok := s.Skills[name]; ok && current.Kind == "delegated" && !opts.Force {
		if current.Repo == repo.Repo &&
			current.Ref == checkout.ResolvedRef &&
			current.Commit == checkout.Commit &&
			current.Version == planned.Version &&
			current.ConfiguredRef == checkout.ConfiguredRef &&
			current.Channel == checkout.Channel &&
			current.FallbackRef == checkout.FallbackRef {
			fmt.Printf("%s already up to date (%s @ %s)\n", name, current.Version, current.Ref)
			return nil
		}
	}
	if err := Uninstall(name); err != nil && !errors.Is(err, errNotInstalled) {
		return err
	}
	return installDelegated(name, repo, opts)
}

func uninstallDelegated(name string, sk state.Skill) error {
	if sk.Repo != "" && sk.Ref != "" {
		repo := config.DelegatedRepo{Repo: sk.Repo, Ref: sk.Ref}
		checkout, err := prepareDelegatedRepo(name, repo)
		if err == nil {
			if installer, err := delegatedInstaller(checkout.Dir); err == nil {
				if _, err := runDelegatedInstaller(installer, name, "uninstall", "all", false, ""); err == nil {
					return removeDelegatedState(name)
				} else {
					fmt.Fprintf(os.Stderr, "warn: delegated uninstall failed for %s, falling back to hash-safe local removal: %v\n", name, err)
				}
			}
		} else {
			fmt.Fprintf(os.Stderr, "warn: delegated repo unavailable for %s, falling back to hash-safe local removal: %v\n", name, err)
		}
	}
	return hashSafeRemoveInstalled(name, sk)
}

func removeDelegatedState(name string) error {
	lock, err := state.Lock()
	if err != nil {
		return err
	}
	defer state.Release(lock)
	s, err := state.Load()
	if err != nil {
		return err
	}
	delete(s.Skills, name)
	if err := state.Save(s); err != nil {
		return err
	}
	_ = state.AppendHistory(state.HistoryEvent{Op: "uninstall", Skill: name})
	fmt.Printf("uninstalled %s\n", name)
	return nil
}

func prepareDelegatedRepo(name string, repo config.DelegatedRepo) (delegatedCheckout, error) {
	cacheRoot, err := delegatedCacheRoot()
	if err != nil {
		return delegatedCheckout{}, err
	}
	repoDir := filepath.Join(cacheRoot, name)
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); errors.Is(err, os.ErrNotExist) {
		if err := os.RemoveAll(repoDir); err != nil {
			return delegatedCheckout{}, err
		}
		if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
			return delegatedCheckout{}, err
		}
		if err := runGit("", "clone", delegatedRepoURL(repo.Repo), repoDir); err != nil {
			return delegatedCheckout{}, err
		}
	} else if err != nil {
		return delegatedCheckout{}, err
	} else {
		if err := runGit(repoDir, "fetch", "--all", "--tags", "--prune"); err != nil {
			return delegatedCheckout{}, err
		}
	}
	checkout, err := resolveDelegatedCheckout(repoDir, repo)
	if err != nil {
		return delegatedCheckout{}, err
	}
	checkout.Dir = repoDir
	if err := runGit(repoDir, "checkout", checkout.ResolvedRef); err != nil {
		return delegatedCheckout{}, err
	}
	if _, err := gitOutput(repoDir, "rev-parse", "--verify", "origin/"+checkout.ResolvedRef); err == nil {
		if err := runGit(repoDir, "reset", "--hard", "origin/"+checkout.ResolvedRef); err != nil {
			return delegatedCheckout{}, err
		}
	}
	commit, err := gitOutput(repoDir, "rev-parse", "HEAD")
	if err != nil {
		return delegatedCheckout{}, fmt.Errorf("git rev-parse HEAD: %w\n%s", err, strings.TrimSpace(string(commit)))
	}
	checkout.Commit = strings.TrimSpace(string(commit))
	return checkout, nil
}

func resolveDelegatedCheckout(repoDir string, repo config.DelegatedRepo) (delegatedCheckout, error) {
	if repo.Ref != "" {
		return delegatedCheckout{
			ResolvedRef:   repo.Ref,
			ConfiguredRef: repo.Ref,
		}, nil
	}
	if repo.Channel != "latest-release" {
		return delegatedCheckout{}, fmt.Errorf("unsupported delegated channel %q", repo.Channel)
	}
	tag, err := latestStableReleaseTag(repoDir)
	if err != nil {
		return delegatedCheckout{}, err
	}
	if tag != "" {
		return delegatedCheckout{
			ResolvedRef:  tag,
			Channel:      repo.Channel,
			FallbackRef:  repo.FallbackRef,
			FallbackUsed: false,
		}, nil
	}
	if repo.FallbackRef != "" {
		return delegatedCheckout{
			ResolvedRef:  repo.FallbackRef,
			Channel:      repo.Channel,
			FallbackRef:  repo.FallbackRef,
			FallbackUsed: true,
		}, nil
	}
	return delegatedCheckout{}, fmt.Errorf("no stable release tags found for %s and no fallback_ref configured", repo.Repo)
}

var stableReleaseTagRE = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)

type releaseTag struct {
	Name       string
	Major, Min int
	Patch      int
}

func latestStableReleaseTag(repoDir string) (string, error) {
	out, err := gitOutput(repoDir, "tag", "--list", "v*")
	if err != nil {
		return "", fmt.Errorf("git tag --list v*: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	var tags []releaseTag
	for _, line := range strings.Split(string(out), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		match := stableReleaseTagRE.FindStringSubmatch(name)
		if match == nil {
			continue
		}
		major, _ := strconv.Atoi(match[1])
		minor, _ := strconv.Atoi(match[2])
		patch, _ := strconv.Atoi(match[3])
		tags = append(tags, releaseTag{Name: name, Major: major, Min: minor, Patch: patch})
	}
	if len(tags) == 0 {
		return "", nil
	}
	sort.Slice(tags, func(i, j int) bool {
		if tags[i].Major != tags[j].Major {
			return tags[i].Major > tags[j].Major
		}
		if tags[i].Min != tags[j].Min {
			return tags[i].Min > tags[j].Min
		}
		return tags[i].Patch > tags[j].Patch
	})
	return tags[0].Name, nil
}

func delegatedCacheRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "mise-en-place", "repos"), nil
}

func delegatedRepoURL(repo string) string {
	if strings.Contains(repo, "://") || strings.HasPrefix(repo, "/") || strings.HasPrefix(repo, ".") {
		return repo
	}
	if strings.HasPrefix(repo, "github.com/") {
		return "https://" + repo
	}
	return repo
}

func delegatedInstaller(repoDir string) (string, error) {
	for _, name := range []string{"install-skill.sh", "install.sh"} {
		path := filepath.Join(repoDir, name)
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("delegated installer not found in %s (expected install-skill.sh or install.sh)", repoDir)
}

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitOutput(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.CombinedOutput()
}

func runDelegatedInstaller(installer, skillName, operation, target string, requireHashes bool, installRoot string) (*delegatedResult, error) {
	args := []string{"--" + operation, "--target", target, "--json"}
	if installRoot != "" {
		absRoot, err := filepath.Abs(installRoot)
		if err != nil {
			return nil, err
		}
		if absRoot, err = filepath.EvalSymlinks(absRoot); err != nil {
			return nil, err
		}
		args = append(args, "--install-root", absRoot)
		installRoot = absRoot
	}
	cmd := exec.Command(installer, args...)
	cmd.Dir = filepath.Dir(installer)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w\n%s", filepath.Base(installer), strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	var result delegatedResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("delegated %s output is not JSON-only: %w", operation, err)
	}
	if err := validateDelegatedResult(&result, skillName, operation, requireHashes); err != nil {
		return nil, err
	}
	if installRoot != "" {
		if err := validateDelegatedPathsInside(&result, skillName, installRoot); err != nil {
			return nil, err
		}
	}
	return &result, nil
}

func validateDelegatedResult(result *delegatedResult, skillName, operation string, requireHashes bool) error {
	if result.Schema != 1 {
		return fmt.Errorf("delegated %s: schema must be 1, got %d", skillName, result.Schema)
	}
	if result.Kind != "delegated" {
		return fmt.Errorf("delegated %s: kind must be delegated, got %q", skillName, result.Kind)
	}
	if result.Name != skillName {
		return fmt.Errorf("delegated %s: result name mismatch %q", skillName, result.Name)
	}
	if result.Operation != operation {
		return fmt.Errorf("delegated %s: operation mismatch %q", skillName, result.Operation)
	}
	if result.Version == "" {
		return fmt.Errorf("delegated %s: version is required", skillName)
	}
	if len(result.Targets) == 0 {
		return fmt.Errorf("delegated %s: targets are required", skillName)
	}
	if err := config.ValidateCapabilities(result.Capabilities, "delegated "+skillName); err != nil {
		return err
	}
	if err := config.ValidateSetupRequirements(result.Setup, result.Capabilities, "delegated "+skillName); err != nil {
		return err
	}
	for targetName, target := range result.Targets {
		for _, file := range target.Files {
			if file.Path == "" || !filepath.IsAbs(file.Path) {
				return fmt.Errorf("delegated %s/%s: file path must be absolute: %q", skillName, targetName, file.Path)
			}
			if requireHashes && file.SHA256 == "" {
				return fmt.Errorf("delegated %s/%s: installed file %s missing sha256", skillName, targetName, file.Path)
			}
		}
	}
	return nil
}

func validateDelegatedPathsInside(result *delegatedResult, skillName, root string) error {
	for targetName, target := range result.Targets {
		for _, file := range target.Files {
			rel, err := filepath.Rel(root, file.Path)
			if err != nil {
				return fmt.Errorf("delegated %s/%s: validate staged path %s: %w", skillName, targetName, file.Path, err)
			}
			if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
				return fmt.Errorf("delegated %s/%s: staged path escapes --install-root: %s", skillName, targetName, file.Path)
			}
		}
	}
	return nil
}

func delegatedStagedPlan(result *delegatedResult, stageRoot string) ([]fileOp, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if home, err = filepath.EvalSymlinks(home); err != nil {
		return nil, err
	}
	var plan []fileOp
	for targetName, target := range result.Targets {
		for _, file := range target.Files {
			rel, err := filepath.Rel(stageRoot, file.Path)
			if err != nil {
				return nil, err
			}
			if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
				return nil, fmt.Errorf("delegated staged path escapes --install-root: %s", file.Path)
			}
			plan = append(plan, fileOp{
				target: targetName,
				src:    file.Path,
				dst:    filepath.Join(home, rel),
				sha256: file.SHA256,
			})
		}
	}
	return plan, nil
}

func delegatedPlan(result *delegatedResult) ([]fileOp, error) {
	var plan []fileOp
	for targetName, target := range result.Targets {
		for _, file := range target.Files {
			plan = append(plan, fileOp{target: targetName, dst: file.Path, sha256: file.SHA256})
		}
	}
	return plan, nil
}

func delegatedTargetNames(targets map[string]delegatedTargetResult) []string {
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
