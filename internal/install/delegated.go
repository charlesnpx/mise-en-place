package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charlesnpx/mise-en-place/internal/config"
	"github.com/charlesnpx/mise-en-place/internal/state"
)

type delegatedResult struct {
	Schema    int                              `json:"schema"`
	Name      string                           `json:"name"`
	Version   string                           `json:"version"`
	Operation string                           `json:"operation"`
	Kind      string                           `json:"kind"`
	Targets   map[string]delegatedTargetResult `json:"targets"`
	Warnings  []string                         `json:"warnings"`
}

type delegatedTargetResult struct {
	Files []delegatedFileResult `json:"files"`
}

type delegatedFileResult struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}

func installDelegated(name string, repo config.DelegatedRepo, opts Options) error {
	target := opts.Target
	if target == "" {
		target = "all"
	}
	repoDir, err := prepareDelegatedRepo(name, repo)
	if err != nil {
		if repo.IsPrivate() {
			return fmt.Errorf("%w; %s is marked private/team-only, check repository access or authentication", err, name)
		}
		return err
	}
	installer, err := delegatedInstaller(repoDir)
	if err != nil {
		return err
	}
	planned, err := runDelegatedInstaller(installer, name, "plan", target, false)
	if err != nil {
		return err
	}
	plan, err := delegatedPlan(planned)
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
	if err := checkUnmanaged(s, name, plan, opts.Backup); err != nil {
		return err
	}

	installed, err := runDelegatedInstaller(installer, name, "install", target, true)
	if err != nil {
		return err
	}
	files, err := delegatedRecordFiles(installed)
	if err != nil {
		return err
	}

	s.Skills[name] = state.Skill{
		Kind:        "delegated",
		Version:     installed.Version,
		Repo:        repo.Repo,
		Ref:         repo.Ref,
		Targets:     files,
		InstalledAt: time.Now().UTC(),
	}
	if err := state.Save(s); err != nil {
		return err
	}
	_ = state.AppendHistory(state.HistoryEvent{
		Op:      "install",
		Skill:   name,
		Version: installed.Version,
		Targets: delegatedTargetNames(installed.Targets),
	})
	for _, warning := range installed.Warnings {
		fmt.Fprintf(os.Stderr, "warn: %s: %s\n", name, warning)
	}
	fmt.Printf("installed %s %s (%s delegated)\n", name, installed.Version, strings.Join(delegatedTargetNames(installed.Targets), ", "))
	return nil
}

func uninstallDelegated(name string, sk state.Skill) error {
	if sk.Repo != "" && sk.Ref != "" {
		repo := config.DelegatedRepo{Repo: sk.Repo, Ref: sk.Ref}
		repoDir, err := prepareDelegatedRepo(name, repo)
		if err == nil {
			if installer, err := delegatedInstaller(repoDir); err == nil {
				if _, err := runDelegatedInstaller(installer, name, "uninstall", "all", false); err == nil {
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

func prepareDelegatedRepo(name string, repo config.DelegatedRepo) (string, error) {
	cacheRoot, err := delegatedCacheRoot()
	if err != nil {
		return "", err
	}
	repoDir := filepath.Join(cacheRoot, name)
	if _, err := os.Stat(filepath.Join(repoDir, ".git")); errors.Is(err, os.ErrNotExist) {
		if err := os.RemoveAll(repoDir); err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(repoDir), 0o755); err != nil {
			return "", err
		}
		if err := runGit("", "clone", delegatedRepoURL(repo.Repo), repoDir); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else {
		if err := runGit(repoDir, "fetch", "--all", "--tags", "--prune"); err != nil {
			return "", err
		}
	}
	if err := runGit(repoDir, "checkout", repo.Ref); err != nil {
		return "", err
	}
	if _, err := gitOutput(repoDir, "rev-parse", "--verify", "origin/"+repo.Ref); err == nil {
		if err := runGit(repoDir, "reset", "--hard", "origin/"+repo.Ref); err != nil {
			return "", err
		}
	}
	return repoDir, nil
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

func runDelegatedInstaller(installer, skillName, operation, target string, requireHashes bool) (*delegatedResult, error) {
	args := []string{"--" + operation, "--target", target, "--json"}
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

func delegatedPlan(result *delegatedResult) ([]fileOp, error) {
	var plan []fileOp
	for targetName, target := range result.Targets {
		for _, file := range target.Files {
			plan = append(plan, fileOp{target: targetName, dst: file.Path, sha256: file.SHA256})
		}
	}
	return plan, nil
}

func delegatedRecordFiles(result *delegatedResult) (map[string]state.TargetRecord, error) {
	out := map[string]state.TargetRecord{}
	for targetName, target := range result.Targets {
		rec := out[targetName]
		for _, file := range target.Files {
			rec.Files = append(rec.Files, state.FileRecord{Path: file.Path, SHA256: file.SHA256})
		}
		out[targetName] = rec
	}
	return out, nil
}

func delegatedTargetNames(targets map[string]delegatedTargetResult) []string {
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
