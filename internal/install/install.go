// Package install implements the managed-skill install pipeline.
//
// Pipeline (under flock):
//  1. Resolve the skill's source (local repo subtree for dev, GitHub Release
//     URL otherwise) and read its skill.yaml.
//  2. Validate min_installer ≤ running CLI version.
//  3. Compute the intended file set across requested targets.
//  4. Refuse cross-skill path collisions against state.json.
//  5. Compare existing files and either skip, prompt, or back them up.
//  6. Copy changed files into place.
//  7. Update state.json (atomic rename), append history.jsonl.
package install

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charlesnpx/mise-en-place/internal/artifact"
	"github.com/charlesnpx/mise-en-place/internal/config"
	"github.com/charlesnpx/mise-en-place/internal/hostpaths"
	"github.com/charlesnpx/mise-en-place/internal/state"
)

// Options carries CLI-level switches into the pipeline.
type Options struct {
	Target           string // "claude" | "codex" | "all"
	Backup           bool
	RunningInstaller string // current CLI version, for min_installer check
	ManifestSchema   int    // schema version this build understands
	// SkillsRoot is the on-disk path to the monorepo's skills/ directory.
	// Defaults to "skills" relative to the working directory.
	SkillsRoot string
	Strict     bool // fail install --all when delegated/optional skills cannot be installed
}

func (o Options) skillsRoot() string {
	if o.SkillsRoot != "" {
		return o.SkillsRoot
	}
	for _, p := range []string{"skills", "../skills", "../../skills"} {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return "skills"
}

// One installs a single skill.
func One(name string, reg *config.Registry, opts Options) error {
	kind := reg.Kind(name)
	if kind == "" {
		return fmt.Errorf("unknown skill: %s (not in registry.yaml)", name)
	}
	if kind == "delegated" {
		return installDelegated(name, reg.Delegated[name], opts)
	}
	return installManaged(name, opts)
}

// All installs every managed skill in the registry. Delegated skills are
// skipped by default when optional/private; --strict turns skipped delegated
// skills into errors.
func All(reg *config.Registry, opts Options) error {
	var errs []string
	for _, name := range reg.Managed {
		if err := installManaged(name, opts); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		}
	}

	dnames := make([]string, 0, len(reg.Delegated))
	for name := range reg.Delegated {
		dnames = append(dnames, name)
	}
	sort.Strings(dnames)
	for _, name := range dnames {
		d := reg.Delegated[name]
		err := installDelegated(name, d, opts)
		if err == nil {
			continue
		}
		if opts.Strict || !d.IsOptional() {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		fmt.Fprintf(os.Stderr, "skip: %s: %v\n", name, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("install --all completed with errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// Upgrade reinstalls a single skill at the current source version.
func Upgrade(name string, reg *config.Registry, opts Options) error {
	if err := Uninstall(name); err != nil && !errors.Is(err, errNotInstalled) {
		return err
	}
	return One(name, reg, opts)
}

// UpgradeAll reinstalls every currently-installed skill.
func UpgradeAll(reg *config.Registry, opts Options) error {
	s, err := state.Load()
	if err != nil {
		return err
	}
	var errs []string
	for name := range s.Skills {
		if reg.Kind(name) == "" {
			continue
		}
		if err := Upgrade(name, reg, opts); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("upgrade --all completed with errors:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

var errNotInstalled = errors.New("skill is not installed")

// Uninstall removes an installed skill from disk and state.
func Uninstall(name string) error {
	s, err := state.Load()
	if err != nil {
		return err
	}
	sk, ok := s.Skills[name]
	if !ok {
		return fmt.Errorf("%w: %s", errNotInstalled, name)
	}
	if sk.Kind == "delegated" {
		return uninstallDelegated(name, sk)
	}

	return hashSafeRemoveInstalled(name, sk)
}

func hashSafeRemoveInstalled(name string, sk state.Skill) error {
	lock, err := state.Lock()
	if err != nil {
		return err
	}
	defer state.Release(lock)

	s, err := state.Load()
	if err != nil {
		return err
	}
	for targetName, tr := range sk.Targets {
		for _, fr := range tr.Files {
			cur, err := artifact.SHA256File(fr.Path)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				fmt.Fprintf(os.Stderr, "warn: hash %s: %v\n", fr.Path, err)
				continue
			}
			if cur != fr.SHA256 {
				fmt.Fprintf(os.Stderr, "warn: leaving locally-modified %s (%s target)\n", fr.Path, targetName)
				continue
			}
			if err := os.Remove(fr.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(os.Stderr, "warn: remove %s: %v\n", fr.Path, err)
			}
			// Try to prune now-empty parent directories under known host roots.
			pruneEmptyParents(fr.Path)
		}
	}

	delete(s.Skills, name)
	if err := state.Save(s); err != nil {
		return err
	}
	_ = state.AppendHistory(state.HistoryEvent{Op: "uninstall", Skill: name})
	fmt.Printf("uninstalled %s\n", name)
	return nil
}

func pruneEmptyParents(filePath string) {
	roots := []string{
		hostpaths.ClaudeCommandsDir(),
		hostpaths.ClaudeSkillsDir(),
		hostpaths.CodexSkillsDir(),
	}
	dir := filepath.Dir(filePath)
	for {
		// Stop if we've walked above any known root.
		within := false
		for _, r := range roots {
			rel, err := filepath.Rel(r, dir)
			if err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
				within = true
				break
			}
		}
		if !within {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

func installManaged(name string, opts Options) error {
	skillDir := filepath.Join(opts.skillsRoot(), name)
	manifestPath := filepath.Join(skillDir, "skill.yaml")
	manifest, err := config.LoadSkillManifest(manifestPath)
	if err != nil {
		return err
	}
	if err := checkMinInstaller(manifest.MinInstaller, opts.RunningInstaller); err != nil {
		return err
	}
	if manifest.ManifestSchema > opts.ManifestSchema {
		return fmt.Errorf("skill %s requires manifest_schema=%d but this build understands %d",
			name, manifest.ManifestSchema, opts.ManifestSchema)
	}

	requested := requestedTargets(manifest, opts.Target)
	if len(requested) == 0 {
		return fmt.Errorf("skill %s declares no targets matching --target=%s", name, opts.Target)
	}

	// Stage outside the lock: build the file plan and hash artifact contents.
	plan, err := buildPlan(skillDir, manifest, requested)
	if err != nil {
		return err
	}

	tarball, err := artifact.TarGzDir(skillDir)
	if err != nil {
		return err
	}
	tarHash := artifact.SHA256Bytes(tarball)
	source := "local:" + mustAbs(skillDir)

	// Critical section.
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
		Kind:           "managed",
		Version:        manifest.Version,
		ManifestSchema: manifest.ManifestSchema,
		Source:         source,
		SHA256:         tarHash,
		Targets:        files,
		InstalledAt:    time.Now().UTC(),
	}
	if err := state.Save(s); err != nil {
		return err
	}
	_ = state.AppendHistory(state.HistoryEvent{
		Op:      "install",
		Skill:   name,
		Version: manifest.Version,
		Targets: targetNames(requested),
	})

	fmt.Printf("installed %s %s (%s)\n", name, manifest.Version, strings.Join(targetNames(requested), ", "))
	if skipped > 0 {
		fmt.Fprintf(os.Stderr, "partial install: skipped %d divergent file(s); skipped files were not recorded in state\n", skipped)
	}
	_ = tarball // hash already captured; tarball not retained on disk in this build
	return nil
}

func requestedTargets(m *config.SkillManifest, want string) map[string]*config.Target {
	out := map[string]*config.Target{}
	for name, t := range m.Targets {
		if want == "all" || want == name {
			out[name] = t
		}
	}
	return out
}

func targetNames(m map[string]*config.Target) []string {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// fileOp describes one file that will be copied into place.
type fileOp struct {
	target string
	src    string
	dst    string
	sha256 string
}

func buildPlan(skillDir string, m *config.SkillManifest, requested map[string]*config.Target) ([]fileOp, error) {
	var ops []fileOp
	for tname, t := range requested {
		srcAbs := filepath.Join(skillDir, t.Payload)
		dst := hostpaths.Expand(t.InstallTo)
		switch t.Type {
		case config.TargetCommand:
			info, err := os.Stat(srcAbs)
			if err != nil {
				return nil, fmt.Errorf("payload missing for %s/%s: %w", m.Name, tname, err)
			}
			if info.IsDir() {
				return nil, fmt.Errorf("%s/%s declares type=command but payload %s is a directory", m.Name, tname, srcAbs)
			}
			sum, err := artifact.SHA256File(srcAbs)
			if err != nil {
				return nil, err
			}
			ops = append(ops, fileOp{target: tname, src: srcAbs, dst: dst, sha256: sum})
		case config.TargetSkillDir:
			info, err := os.Stat(srcAbs)
			if err != nil {
				return nil, fmt.Errorf("payload missing for %s/%s: %w", m.Name, tname, err)
			}
			if !info.IsDir() {
				return nil, fmt.Errorf("%s/%s declares type=skill_dir but payload %s is not a directory", m.Name, tname, srcAbs)
			}
			err = filepath.WalkDir(srcAbs, func(path string, d fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if d.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(srcAbs, path)
				if err != nil {
					return err
				}
				sum, err := artifact.SHA256File(path)
				if err != nil {
					return err
				}
				ops = append(ops, fileOp{
					target: tname,
					src:    path,
					dst:    filepath.Join(dst, rel),
					sha256: sum,
				})
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
	}
	return ops, nil
}

func checkCollisions(s *state.State, self string, plan []fileOp) error {
	owners := map[string]string{}
	for skillName, sk := range s.Skills {
		if skillName == self {
			continue
		}
		for _, tr := range sk.Targets {
			for _, fr := range tr.Files {
				owners[fr.Path] = skillName
			}
		}
	}
	for _, op := range plan {
		if owner, taken := owners[op.dst]; taken {
			return fmt.Errorf("path collision: %s already owns %s (resolve by editing skill.yaml in git)", owner, op.dst)
		}
	}
	return nil
}

func checkDestinationConflicts(s *state.State, self string, plan []fileOp) error {
	owned := map[string]bool{}
	if sk, ok := s.Skills[self]; ok {
		for _, tr := range sk.Targets {
			for _, fr := range tr.Files {
				owned[fr.Path] = true
			}
		}
	}
	for _, op := range plan {
		if _, err := os.Stat(op.dst); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		if op.sha256 != "" {
			currentHash, err := artifact.SHA256File(op.dst)
			if err != nil {
				return fmt.Errorf("hash existing file %s: %w", op.dst, err)
			}
			if currentHash == op.sha256 {
				continue
			}
		} else if owned[op.dst] {
			continue
		}
		if owned[op.dst] {
			return fmt.Errorf("owned file differs at %s", op.dst)
		}
		return fmt.Errorf("existing unmanaged file differs at %s", op.dst)
	}
	return nil
}

func applyOwnershipPlan(s *state.State, self string, plan []fileOp, backup bool) ([]fileOp, int, error) {
	owned := map[string]bool{}
	if sk, ok := s.Skills[self]; ok {
		for _, tr := range sk.Targets {
			for _, fr := range tr.Files {
				owned[fr.Path] = true
			}
		}
	}
	var applied []fileOp
	skipped := 0
	for _, op := range plan {
		if _, err := os.Stat(op.dst); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				if err := installPlannedFile(op); err != nil {
					return nil, skipped, err
				}
				applied = append(applied, op)
				continue
			}
			return nil, skipped, err
		}

		currentHash, err := artifact.SHA256File(op.dst)
		if err != nil {
			return nil, skipped, fmt.Errorf("hash existing file %s: %w", op.dst, err)
		}
		if currentHash == op.sha256 {
			fmt.Printf("Skipping, skill up to date: %s\n", op.dst)
			applied = append(applied, op)
			continue
		}

		if backup {
			if err := backupExisting(op.dst); err != nil {
				return nil, skipped, err
			}
			if err := installPlannedFile(op); err != nil {
				return nil, skipped, err
			}
			applied = append(applied, op)
			continue
		}

		if !isInteractiveStdin() {
			ownerHint := "existing"
			if !owned[op.dst] {
				ownerHint = "unmanaged existing"
			}
			return nil, skipped, fmt.Errorf("%s file differs at %s (run in a terminal to choose overwrite, backup, or skip; or pass --backup to back up and overwrite)", ownerHint, op.dst)
		}

		printPlannedDiff(op)
		action, err := promptDivergentAction(op.dst)
		if err != nil {
			return nil, skipped, err
		}
		switch action {
		case "overwrite":
			if err := installPlannedFile(op); err != nil {
				return nil, skipped, err
			}
			applied = append(applied, op)
		case "backup":
			if err := backupExisting(op.dst); err != nil {
				return nil, skipped, err
			}
			if err := installPlannedFile(op); err != nil {
				return nil, skipped, err
			}
			applied = append(applied, op)
		case "skip":
			fmt.Fprintf(os.Stderr, "skipped divergent file: %s\n", op.dst)
			skipped++
			continue
		default:
			return nil, skipped, fmt.Errorf("unknown install action %q", action)
		}
	}
	return applied, skipped, nil
}

func installPlannedFile(op fileOp) error {
	if err := os.MkdirAll(filepath.Dir(op.dst), 0o755); err != nil {
		return err
	}
	if err := copyFile(op.src, op.dst); err != nil {
		return fmt.Errorf("copy %s -> %s: %w", op.src, op.dst, err)
	}
	return nil
}

func backupExisting(path string) error {
	bak := nextBackupPath(path)
	if err := os.Rename(path, bak); err != nil {
		return fmt.Errorf("backup %s: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "backed up existing %s -> %s\n", path, bak)
	return nil
}

func nextBackupPath(path string) string {
	base := path + ".backup"
	if _, err := os.Stat(base); errors.Is(err, os.ErrNotExist) {
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.%d", base, i)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func isInteractiveStdin() bool {
	info, err := os.Stdin.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

func printPlannedDiff(op fileOp) {
	fmt.Fprintf(os.Stderr, "Existing file differs: %s\n", op.dst)
	out, err := exec.Command("diff", "-u", "--label", "existing", "--label", "planned", op.dst, op.src).CombinedOutput()
	if len(out) > 0 {
		fmt.Fprint(os.Stderr, string(out))
		return
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "diff unavailable for %s: %v\n", op.dst, err)
		return
	}
	fmt.Fprintln(os.Stderr, "(files differ, but no text diff was produced)")
}

func promptDivergentAction(path string) (string, error) {
	for {
		fmt.Fprintf(os.Stderr, "Choose action for %s [overwrite/backup/skip]: ", path)
		var answer string
		if _, err := fmt.Fscan(os.Stdin, &answer); err != nil {
			return "", err
		}
		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "overwrite", "o":
			return "overwrite", nil
		case "backup", "b":
			return "backup", nil
		case "skip", "s":
			return "skip", nil
		default:
			fmt.Fprintln(os.Stderr, "Please type overwrite, backup, or skip.")
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	srcInfo, err := in.Stat()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".install-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, srcInfo.Mode().Perm()); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}

func recordFiles(plan []fileOp) (map[string]state.TargetRecord, error) {
	out := map[string]state.TargetRecord{}
	for _, op := range plan {
		t := out[op.target]
		t.Files = append(t.Files, state.FileRecord{Path: op.dst, SHA256: op.sha256})
		out[op.target] = t
	}
	return out, nil
}

func mustAbs(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return abs
}

// PrintList writes a human-readable view of installed and available skills.
func PrintList(w io.Writer, s *state.State, reg *config.Registry) {
	fmt.Fprintln(w, "Installed:")
	if len(s.Skills) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	names := make([]string, 0, len(s.Skills))
	for n := range s.Skills {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		sk := s.Skills[n]
		targets := make([]string, 0, len(sk.Targets))
		for t := range sk.Targets {
			targets = append(targets, t)
		}
		sort.Strings(targets)
		extra := ""
		if sk.Kind == "delegated" {
			extra = " [delegated]"
		}
		if sk.Adopted {
			extra += " [adopted]"
		}
		fmt.Fprintf(w, "  %s %s (%s)%s\n", n, sk.Version, strings.Join(targets, ","), extra)
	}

	fmt.Fprintln(w, "\nAvailable in registry:")
	for _, n := range reg.Managed {
		marker := "  "
		if _, ok := s.Skills[n]; ok {
			marker = "✓ "
		}
		fmt.Fprintf(w, "%s%s (managed)\n", marker, n)
	}
	dnames := make([]string, 0, len(reg.Delegated))
	for n := range reg.Delegated {
		dnames = append(dnames, n)
	}
	sort.Strings(dnames)
	for _, n := range dnames {
		marker := "  "
		if _, ok := s.Skills[n]; ok {
			marker = "✓ "
		}
		d := reg.Delegated[n]
		attrs := []string{"delegated"}
		if d.Visibility != "" {
			attrs = append(attrs, d.Visibility)
		}
		if d.Optional {
			attrs = append(attrs, "optional")
		}
		fmt.Fprintf(w, "%s%s (%s %s@%s)\n", marker, n, strings.Join(attrs, ", "), d.Repo, d.Ref)
	}
}
