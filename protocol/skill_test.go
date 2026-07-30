package protocol_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalSkillContainsCompleteRunnerNeutralContract(t *testing.T) {
	files := []string{
		"higurashi-deliver/SKILL.md",
		"higurashi-deliver/references/artifact-contract.md",
		"higurashi-deliver/references/reviewer-contract.md",
	}
	var combined strings.Builder
	for _, name := range files {
		content, err := os.ReadFile(filepath.FromSlash(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		combined.Write(content)
		combined.WriteByte('\n')
	}
	text := combined.String()

	required := []string{
		"REFINE",
		"PLAN",
		"APPLY",
		"VERIFY",
		"higurashi inspect",
		"higurashi transition",
		"higurashi repair authorize",
		"Repair-Round",
		"Handoff-Status",
		"Candidate-Strategy: uncommitted",
		"minimum-acceptance:",
		"maxApplyBatchesPerRun",
		"maxRepairAttempts",
		"CodeGraph",
		"one task",
		"`Permitted commands:`",
		"For a legacy task",
		"failing or passing evidence",
		"higurashi verification suggest --json",
		"verification.requiredCommands",
		"Configured commands are command authority",
		"Do not block for a missing task-local command when an applicable configured command covers the required evidence.",
		"Run `higurashi verification suggest --json` only when no configured command covers the missing evidence.",
		"A clear retry reply",
		"restart PRECHECK for the same work-item ID and original runner options",
		"A retry never authorizes a repair round",
		"`repair_plan_required`",
		"invoke PLAN exactly once",
		"repair authorization while repair planning is required",
		"require `repair_ready`",
		"user-owned",
		"contract reviewer",
		"risk reviewer",
		"Do not alter control documents to manufacture success.",
		"Repeat the anti-bypass rule in every subagent prompt.",
	}
	for _, value := range required {
		if !strings.Contains(text, value) {
			t.Errorf("canonical protocol is missing %q", value)
		}
	}

	forbidden := []string{
		"OpenCode",
		"Claude Code",
		"model:",
		"permission:",
		"/home/",
	}
	for _, value := range forbidden {
		if strings.Contains(strings.ToLower(text), strings.ToLower(value)) {
			t.Errorf("canonical protocol contains forbidden term %q", value)
		}
	}
}

func TestCanonicalSkillFrontmatterIsPortableAndMinimal(t *testing.T) {
	content, err := os.ReadFile("higurashi-deliver/SKILL.md")
	if err != nil {
		t.Fatalf("read canonical skill: %v", err)
	}
	parts := strings.SplitN(string(content), "---\n", 3)
	if len(parts) != 3 || parts[0] != "" {
		t.Fatal("canonical skill must begin with YAML frontmatter")
	}
	var keys []string
	for _, line := range strings.Split(strings.TrimSpace(parts[1]), "\n") {
		key, _, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("invalid frontmatter line %q", line)
		}
		keys = append(keys, key)
	}
	if strings.Join(keys, ",") != "name,description" {
		t.Errorf("frontmatter keys = %v, want name and description only", keys)
	}
	if lines := strings.Count(string(content), "\n") + 1; lines > 500 {
		t.Errorf("canonical skill has %d lines, want at most 500", lines)
	}
}

func TestCanonicalRefineSkillRequiresConfirmedBoundedRefinement(t *testing.T) {
	content, err := os.ReadFile("higurashi-refine/SKILL.md")
	if err != nil {
		t.Fatalf("read canonical refine skill: %v", err)
	}
	text := string(content)
	required := []string{
		"name: higurashi-refine",
		"higurashi inspect",
		"kind: ready",
		"one batch",
		"recommended answer",
		"at most one follow-up",
		"free-text",
		"explicit confirmation",
		"Status: refined",
		"Repair-Round: 0",
		"Never modify the requirement source",
		"Do not alter control documents",
	}
	for _, value := range required {
		if !strings.Contains(text, value) {
			t.Errorf("canonical refine protocol is missing %q", value)
		}
	}
	forbidden := []string{
		"OpenCode",
		"Claude Code",
		"model:",
		"permission:",
		"/home/",
	}
	for _, value := range forbidden {
		if strings.Contains(strings.ToLower(text), strings.ToLower(value)) {
			t.Errorf("canonical refine protocol contains forbidden term %q", value)
		}
	}
}
