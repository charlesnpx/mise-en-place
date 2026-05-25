package miseenplace

import "testing"

func TestBundledIncludesNestedPrSkillPayloads(t *testing.T) {
	for _, path := range []string{
		"skills/pr/claude/pr__colon__description/SKILL.md",
		"skills/pr/claude/pr__colon__review__colon__stack-local/SKILL.md",
		"skills/pr/codex/pr__colon__description/SKILL.md",
		"skills/pr/codex/pr__colon__review__colon__stack-local/SKILL.md",
	} {
		if _, err := Bundled.ReadFile(path); err != nil {
			t.Fatalf("bundled payload %s missing: %v", path, err)
		}
	}
}
