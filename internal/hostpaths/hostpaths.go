// Package hostpaths resolves the install destinations on disk for Claude
// Code and Codex CLI. Tilde-expansion happens here.
package hostpaths

import (
	"os"
	"path/filepath"
	"strings"
)

// Expand resolves ~ and ~/ to the user's home directory.
func Expand(p string) string {
	if p == "" {
		return p
	}
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return home
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}

// ClaudeCommandsDir returns ~/.claude/commands.
func ClaudeCommandsDir() string {
	return Expand("~/.claude/commands")
}

// ClaudeSkillsDir returns ~/.claude/skills.
func ClaudeSkillsDir() string {
	return Expand("~/.claude/skills")
}

// CodexSkillsDir returns ~/.codex/skills.
func CodexSkillsDir() string {
	return Expand("~/.codex/skills")
}

// HelpersDir returns ~/.local/share/mise-en-place/skills/<skill>/helpers.
func HelpersDir(skillName string) string {
	return Expand(filepath.Join("~/.local/share/mise-en-place/skills", skillName, "helpers"))
}
