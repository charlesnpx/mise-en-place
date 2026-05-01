// Command mise-en-place manages Claude Code and Codex CLI skills.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	mep "github.com/charlesnpx/mise-en-place"
	"github.com/charlesnpx/mise-en-place/internal/config"
	"github.com/charlesnpx/mise-en-place/internal/install"
	"github.com/charlesnpx/mise-en-place/internal/state"
)

// Version is set via -ldflags at release time.
var Version = "dev"

// ManifestSchema is the schema version this build understands.
const ManifestSchema = 1

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "mise-en-place",
		Short:         "Skill manager for Claude Code and Codex CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		newVersionCmd(),
		newListCmd(),
		newInstallCmd(),
		newUninstallCmd(),
		newUpgradeCmd(),
		newPortCmd(),
		newPortAlignCmd(),
		newDoctorCmd(),
		newScanCmd(),
		newAdoptCmd(),
	)
	return root
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the mise-en-place version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("mise-en-place %s (manifest_schema=%d)\n", Version, ManifestSchema)
			return nil
		},
	}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed and available skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := state.Load()
			if err != nil {
				return err
			}
			source, err := loadRegistry()
			if err != nil {
				return err
			}
			install.PrintList(os.Stdout, s, source.Registry)
			return nil
		},
	}
}

func newInstallCmd() *cobra.Command {
	var (
		target string
		all    bool
		backup bool
		strict bool
	)
	cmd := &cobra.Command{
		Use:   "install [skill]",
		Short: "Install a skill (or --all)",
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := loadRegistry()
			if err != nil {
				return err
			}
			opts := install.Options{
				Target:           target,
				Backup:           backup,
				RunningInstaller: Version,
				ManifestSchema:   ManifestSchema,
				SkillsRoot:       source.SkillsRoot,
				Strict:           strict,
			}
			if all {
				return install.All(source.Registry, opts)
			}
			if len(args) != 1 {
				return errors.New("specify a skill name or --all")
			}
			return install.One(args[0], source.Registry, opts)
		},
	}
	cmd.Flags().StringVar(&target, "target", "all", "claude | codex | all")
	cmd.Flags().BoolVar(&all, "all", false, "install every skill in the registry")
	cmd.Flags().BoolVar(&backup, "backup", false, "rename pre-existing files to *.mise-en-place.bak.<ts>")
	cmd.Flags().BoolVar(&strict, "strict", false, "fail install --all if optional/private delegated skills cannot be installed")
	return cmd
}

func newUninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <skill>",
		Short: "Uninstall a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return install.Uninstall(args[0])
		},
	}
}

func newUpgradeCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "upgrade [skill]",
		Short: "Upgrade an installed skill (or --all)",
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := loadRegistry()
			if err != nil {
				return err
			}
			opts := install.Options{
				Target:           "all",
				RunningInstaller: Version,
				ManifestSchema:   ManifestSchema,
				SkillsRoot:       source.SkillsRoot,
			}
			if all {
				return install.UpgradeAll(source.Registry, opts)
			}
			if len(args) != 1 {
				return errors.New("specify a skill name or --all")
			}
			return install.Upgrade(args[0], source.Registry, opts)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "upgrade every installed skill")
	return cmd
}

func newPortCmd() *cobra.Command {
	var from, to string
	cmd := &cobra.Command{
		Use:   "port <skill>",
		Short: "Draft a translation of a skill from one host to another",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return errStub("port")
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "source host (claude | codex)")
	cmd.Flags().StringVar(&to, "to", "", "target host (claude | codex)")
	_ = cmd.MarkFlagRequired("from")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func newPortAlignCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "port-align <skill>",
		Short: "Recompute payload tree hashes and write skills/<skill>/.alignment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return errStub("port-align")
		},
	}
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report drift, integrity, and collision issues",
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := loadRegistry()
			if err != nil {
				return err
			}
			return install.Doctor(os.Stdout, source.Registry, install.Options{
				Target:           "all",
				RunningInstaller: Version,
				ManifestSchema:   ManifestSchema,
				SkillsRoot:       source.SkillsRoot,
			})
		},
	}
}

func newScanCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Detect unmanaged skill-like files at known install paths",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errStub("scan")
		},
	}
}

func newAdoptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "adopt <skill>",
		Short: "Adopt an existing on-disk file as managed",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return errStub("adopt")
		},
	}
}

func errStub(name string) error {
	return fmt.Errorf("%s is not yet implemented in this build", name)
}

type registrySource struct {
	Registry   *config.Registry
	SkillsRoot string
}

func loadRegistry() (*registrySource, error) {
	if home := os.Getenv("MISE_EN_PLACE_HOME"); home != "" {
		return loadRegistryFromRoot(home)
	}
	for _, root := range []string{".", "..", "../.."} {
		if _, err := os.Stat(filepath.Join(root, "registry.yaml")); err == nil {
			return loadRegistryFromRoot(root)
		}
	}
	root, err := defaultHome()
	if err != nil {
		return nil, err
	}
	if err := materializeBundledHome(root); err != nil {
		return nil, err
	}
	return loadRegistryFromRoot(root)
}

func loadRegistryFromRoot(root string) (*registrySource, error) {
	registryPath := filepath.Join(root, "registry.yaml")
	reg, err := config.LoadRegistry(registryPath)
	if err != nil {
		return nil, err
	}
	return &registrySource{
		Registry:   reg,
		SkillsRoot: filepath.Join(root, "skills"),
	}, nil
}

func defaultHome() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mise-en-place"), nil
}

func materializeBundledHome(root string) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(root, "skills")); err != nil {
		return err
	}
	for _, path := range []string{"registry.yaml", "skills"} {
		if err := fs.WalkDir(mep.Bundled, path, func(src string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			dst := filepath.Join(root, src)
			if d.IsDir() {
				return os.MkdirAll(dst, 0o755)
			}
			body, err := mep.Bundled.ReadFile(src)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			return os.WriteFile(dst, body, 0o644)
		}); err != nil {
			return err
		}
	}
	return nil
}
