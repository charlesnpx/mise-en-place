// Command mise-en-place manages Claude Code and Codex CLI skills.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

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
			reg, err := loadRegistry()
			if err != nil {
				return err
			}
			install.PrintList(os.Stdout, s, reg)
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
			reg, err := loadRegistry()
			if err != nil {
				return err
			}
			opts := install.Options{
				Target:           target,
				Backup:           backup,
				RunningInstaller: Version,
				ManifestSchema:   ManifestSchema,
				Strict:           strict,
			}
			if all {
				return install.All(reg, opts)
			}
			if len(args) != 1 {
				return errors.New("specify a skill name or --all")
			}
			return install.One(args[0], reg, opts)
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
			reg, err := loadRegistry()
			if err != nil {
				return err
			}
			opts := install.Options{
				Target:           "all",
				RunningInstaller: Version,
				ManifestSchema:   ManifestSchema,
			}
			if all {
				return install.UpgradeAll(reg, opts)
			}
			if len(args) != 1 {
				return errors.New("specify a skill name or --all")
			}
			return install.Upgrade(args[0], reg, opts)
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
			reg, err := loadRegistry()
			if err != nil {
				return err
			}
			return install.Doctor(os.Stdout, reg, install.Options{
				Target:           "all",
				RunningInstaller: Version,
				ManifestSchema:   ManifestSchema,
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

func loadRegistry() (*config.Registry, error) {
	// In a release binary the registry is embedded; for development we read
	// it from the working directory or a discoverable repo root.
	for _, p := range []string{"registry.yaml", "../registry.yaml", "../../registry.yaml"} {
		if _, err := os.Stat(p); err == nil {
			return config.LoadRegistry(p)
		}
	}
	return nil, errors.New("registry.yaml not found (run from within the mise-en-place repo, or set MISE_EN_PLACE_HOME)")
}
