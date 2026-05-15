package install

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charlesnpx/mise-en-place/internal/config"
	"github.com/charlesnpx/mise-en-place/internal/state"
)

func delegatedPipxToolsRequested(target string) bool {
	return target == "" || target == "all" || target == "claude" || target == "codex" || target == "tools"
}

func installDelegatedPipxTools(name string, specs []config.DelegatedToolSpec, checkout delegatedCheckout) ([]state.DelegatedTool, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	pipx, err := exec.LookPath("pipx")
	if err != nil {
		return nil, fmt.Errorf("delegated %s tools: pipx not found; install with: brew install pipx && pipx ensurepath", name)
	}

	records := make([]state.DelegatedTool, 0, len(specs))
	for _, spec := range specs {
		installSource := checkout.Dir
		cmd := exec.Command(pipx, "install", "--force", installSource)
		out, err := cmd.CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			return nil, fmt.Errorf("delegated %s tools: pipx install --force %s failed: %s", name, installSource, msg)
		}
		path, ok := externalToolPath(spec.Executable)
		if !ok {
			return nil, fmt.Errorf("delegated %s tools: %s installed with pipx but executable %s is not on PATH; run: pipx ensurepath", name, spec.Package, spec.Executable)
		}
		now := time.Now().UTC()
		records = append(records, state.DelegatedTool{
			Executable:    spec.Executable,
			Path:          path,
			Manager:       spec.Manager,
			Package:       spec.Package,
			InstallSource: installSource,
			Installed:     true,
			InstalledAt:   now,
			VerifiedAt:    now,
		})
		fmt.Printf("installed delegated tool %s via pipx at %s\n", spec.Executable, path)
	}
	return records, nil
}

func uninstallDelegatedPipxTools(skillName string, tools []state.DelegatedTool) {
	if len(tools) == 0 {
		return
	}
	pipx, err := exec.LookPath("pipx")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: delegated %s tools: pipx not found; leaving pipx packages installed\n", skillName)
		return
	}
	for _, tool := range tools {
		pkg := tool.Package
		if pkg == "" {
			pkg = tool.Executable
		}
		cmd := exec.Command(pipx, "uninstall", pkg)
		out, err := cmd.CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			fmt.Fprintf(os.Stderr, "warn: delegated %s tools: pipx uninstall %s failed: %s\n", skillName, pkg, msg)
			continue
		}
		fmt.Printf("uninstalled delegated tool %s via pipx\n", tool.Executable)
	}
}

func delegatedPipxToolsCurrent(specs []config.DelegatedToolSpec, records []state.DelegatedTool) bool {
	if len(specs) == 0 {
		return true
	}
	byExecutable := map[string]state.DelegatedTool{}
	for _, record := range records {
		byExecutable[record.Executable] = record
	}
	for _, spec := range specs {
		record, ok := byExecutable[spec.Executable]
		if !ok {
			return false
		}
		if record.Manager != spec.Manager || record.Package != spec.Package {
			return false
		}
		if _, ok := externalToolPath(spec.Executable); !ok {
			return false
		}
	}
	return true
}

func delegatedProjectVersion(repoDir, fallback string) string {
	body, err := os.ReadFile(repoDir + "/pyproject.toml")
	if err != nil {
		return fallback
	}
	inProject := false
	for _, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(raw)
		if line == "[project]" {
			inProject = true
			continue
		}
		if strings.HasPrefix(line, "[") {
			inProject = false
		}
		if !inProject || !strings.HasPrefix(line, "version") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		if value != "" {
			return value
		}
	}
	return fallback
}
