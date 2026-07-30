package render_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/render"
)

func TestBuildConfiguredAddsRoleSpecificOpenCodeModels(t *testing.T) {
	models := config.InheritedModels()
	models.Orchestrator = "openai/control-model"
	models.Apply = "openai/coding-model#max"
	models.VerifyRisk = "other/risk-model"

	bundle, err := render.BuildConfigured("opencode", "test", models)
	if err != nil {
		t.Fatalf("BuildConfigured: %v", err)
	}
	wants := map[string]string{
		".opencode/agents/higurashi-orchestrator.md": "model: openai/control-model\n",
		".opencode/agents/higurashi-apply.md":        "model: openai/coding-model#max\n",
		".opencode/agents/higurashi-verify-risk.md":  "model: other/risk-model\n",
	}
	for _, file := range bundle.Files {
		want, ok := wants[file.Path]
		if ok && !bytes.Contains(file.Content, []byte(want)) {
			t.Errorf("%s does not contain %q", file.Path, want)
		}
		if file.Path == ".opencode/agents/higurashi-plan.md" &&
			bytes.Contains(file.Content, []byte("\nmodel: ")) {
			t.Error("inherited plan model was rendered as an explicit model")
		}
	}
}

func TestBuildIsDeterministicAndCarriesGeneratedOwnership(t *testing.T) {
	first, err := render.Build("opencode", "1.2.3")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	second, err := render.Build("opencode", "1.2.3")
	if err != nil {
		t.Fatalf("second Build() error = %v", err)
	}

	if len(first.Files) != 12 {
		t.Fatalf("len(bundle.Files) = %d, want 12", len(first.Files))
	}
	for index, file := range first.Files {
		if file.Path != second.Files[index].Path ||
			string(file.Content) != string(second.Files[index].Content) {
			t.Errorf("rendered file %d is not deterministic", index)
		}
		sourceHash := fmt.Sprintf("%x", sha256.Sum256(file.Source))
		if file.SourceHash != sourceHash {
			t.Errorf(
				"file %s source hash = %q, want %q",
				file.Path,
				file.SourceHash,
				sourceHash,
			)
		}
		header := string(file.Content)
		for _, value := range []string{
			"generator=higurashi",
			"version=1.2.3",
			"template=" + file.TemplateID,
			"source-sha256=" + file.SourceHash,
		} {
			if !strings.Contains(header, value) {
				t.Errorf("file %s header is missing %q", file.Path, value)
			}
		}
	}
}

func TestOpenCodeBundleMatchesGoldenSources(t *testing.T) {
	bundle, err := render.Build("opencode", "test")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var actual []string
	for _, file := range bundle.Files {
		if strings.HasPrefix(file.TemplateID, "adapters/opencode/") {
			actual = append(
				actual,
				fmt.Sprintf(
					"%s  %s  %s",
					file.SourceHash,
					file.Path,
					file.TemplateID,
				),
			)
		}
	}
	golden, err := os.ReadFile("testdata/opencode.golden")
	if err != nil {
		t.Fatalf("read OpenCode golden file: %v", err)
	}
	if strings.Join(actual, "\n")+"\n" != string(golden) {
		t.Errorf(
			"OpenCode rendered sources differ from golden file\nactual:\n%s",
			strings.Join(actual, "\n"),
		)
	}
}

func TestOpenCodeRoleContracts(t *testing.T) {
	bundle, err := render.Build("opencode", "test")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	files := make(map[string]string, len(bundle.Files))
	for _, file := range bundle.Files {
		files[file.Path] = string(file.Source)
	}

	command := files[".opencode/commands/higurashi-deliver.md"]
	requireContains(t, command,
		"agent: higurashi-orchestrator",
		"subtask: false",
		"$ARGUMENTS",
		"--plan-only",
		"--repair",
	)

	refineCommand := files[".opencode/commands/higurashi-refine.md"]
	requireContains(t, refineCommand,
		"agent: higurashi-refine",
		"subtask: false",
		"$ARGUMENTS",
		"optional `--from <project-relative-path>`",
	)

	refiner := files[".opencode/agents/higurashi-refine.md"]
	requireContains(t, refiner,
		"mode: primary",
		"higurashi-refine: allow",
		"at most one follow-up",
		"recommended answer",
		"explicit confirmation",
		"Status: refined",
		"higurashi requirements import *",
		"sole permitted requirement/configuration mutation",
	)

	orchestrator := files[".opencode/agents/higurashi-orchestrator.md"]
	requireContains(t, orchestrator,
		"mode: primary",
		"edit: ask",
		"\"*\": deny",
		"higurashi-plan: allow",
		"higurashi-apply: allow",
		"higurashi-verify-contract: allow",
		"higurashi-verify-risk: allow",
		"loop.maxApplyBatchesPerRun",
		"loop.maxRepairAttempts",
		"For other `resume` states, never invoke",
		"exactly the returned `nextTask`",
		"higurashi config validate --json",
		"higurashi verification suggest --json",
		"`Permitted commands:`",
		"artifact path and hash",
		"allowed scope",
		"relevant instructions",
		"For a legacy task",
		"failing or passing evidence",
		"verification.requiredCommands",
		"Configured commands are command authority",
		"Do not block for a missing task-local command when an applicable configured command covers the required evidence.",
		"Run `higurashi verification suggest --json` only when no configured command covers the missing evidence.",
		"A clear retry reply",
		"restart PRECHECK for the same work-item ID and original runner options",
		"A retry never authorizes a repair round",
		"user-owned",
		"higurashi verification suggest *",
		"higurashi repair authorize *",
		"exact expected versioned repair\nhandoff",
		"artifact status is `refined`",
		"/higurashi-refine",
		"Classify each user message before starting delivery",
		"Answer questions directly",
		"propose concrete changes",
		"Do not require a work-item invocation",
		"to discuss, inspect, or propose",
		"Start delivery only for",
		"read-only tools",
		"Do not invoke Higurashi subagents",
		"run workflow-state mutations",
		"or edit files\nin conversation mode",
		"`repair_plan_required`",
		"invoke PLAN exactly once",
		"repair authorization while repair planning is required",
		"require `repair_ready`",
	)
	if strings.Contains(orchestrator, "Reject every other") {
		t.Error("orchestrator rejects conversational input instead of answering it")
	}

	planner := files[".opencode/agents/higurashi-plan.md"]
	requireContains(t, planner,
		"mode: subagent",
		"hidden: true",
		"edit: ask",
		"task: deny",
		"For `kind: ready`",
		"For `kind: repair_plan_required`",
		"repair-r<round>-<blocker-id>",
		"artifact status `refined`",
		"refined` to `planned",
		"`Permitted commands:`",
		"Do not implement",
	)

	apply := files[".opencode/agents/higurashi-apply.md"]
	requireContains(t, apply,
		"mode: subagent",
		"edit: allow",
		"task: deny",
		"exactly the single task",
		"Run only commands in that exact list",
		"Never delegate",
		"git commit*\": deny",
		"git push*\": deny",
		"higurashi repair authorize *\": deny",
		"repair handoffs",
	)

	for _, name := range []string{
		".opencode/agents/higurashi-verify-contract.md",
		".opencode/agents/higurashi-verify-risk.md",
	} {
		reviewer := files[name]
		requireContains(t, reviewer,
			"mode: subagent",
			"\"*\": deny",
			"read: allow",
			"edit: deny",
			"bash: deny",
			"task: deny",
			"Remain read-only and never delegate",
			"minimum-acceptance:",
		)
	}

	for name, content := range files {
		if !strings.HasPrefix(name, ".opencode/agents/") {
			continue
		}
		requireContains(t, content, render.AntiBypassRule, "CodeGraph before broad")
		if strings.Contains(content, "\nmodel:") ||
			strings.Contains(content, "\nreasoningEffort:") {
			t.Errorf("%s pins model configuration", name)
		}
	}
	for name := range files {
		if name == "opencode.json" || name == ".opencode/opencode.json" {
			t.Errorf("bundle modifies default OpenCode configuration: %s", name)
		}
	}
}

func TestClaudeBundleContainsPluginAndStandaloneVariants(t *testing.T) {
	bundle, err := render.Build("claude-code", "test")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(bundle.Files) != 18 {
		t.Fatalf("len(bundle.Files) = %d, want 18", len(bundle.Files))
	}
	for _, file := range bundle.Files {
		if strings.HasPrefix(file.Path, ".opencode/") ||
			strings.HasPrefix(file.TemplateID, "adapters/opencode/") {
			t.Errorf("Claude bundle contains OpenCode file %s", file.Path)
		}
	}
}

func TestClaudeBundleMatchesGoldenSources(t *testing.T) {
	bundle, err := render.Build("claude-code", "test")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	var actual []string
	for _, file := range bundle.Files {
		if strings.HasPrefix(file.TemplateID, "adapters/claude-code/") {
			actual = append(
				actual,
				fmt.Sprintf(
					"%s  %s  %s",
					file.SourceHash,
					file.Path,
					file.TemplateID,
				),
			)
		}
	}
	golden, err := os.ReadFile("testdata/claude-code.golden")
	if err != nil {
		t.Fatalf("read Claude Code golden file: %v", err)
	}
	if strings.Join(actual, "\n")+"\n" != string(golden) {
		t.Errorf(
			"Claude Code rendered sources differ from golden file\nactual:\n%s",
			strings.Join(actual, "\n"),
		)
	}
}

func TestClaudePluginAndMCPContracts(t *testing.T) {
	bundle, err := render.Build("claude-code", "test")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	files := make(map[string]string, len(bundle.Files))
	for _, file := range bundle.Files {
		files[file.Path] = string(file.Content)
	}

	var plugin struct {
		Author struct {
			Name string `json:"name"`
		} `json:"author"`
		Description string `json:"description"`
		Name        string `json:"name"`
		Version     string `json:"version"`
	}
	if err := json.Unmarshal(
		[]byte(files[".claude-plugin/plugin.json"]),
		&plugin,
	); err != nil {
		t.Fatalf("decode generated plugin manifest: %v", err)
	}
	if plugin.Name != "higurashi-loop" || plugin.Version != "0.1.0" {
		t.Errorf("plugin identity = %q %q", plugin.Name, plugin.Version)
	}
	if plugin.Author.Name != "Higurashi Loop contributors" {
		t.Errorf("plugin author = %q", plugin.Author.Name)
	}
	if !strings.Contains(plugin.Description, "Higurashi-Generated:") {
		t.Error("plugin description is missing generated ownership")
	}

	var mcp struct {
		Servers map[string]struct {
			Type    string            `json:"type"`
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(files[".mcp.json"]), &mcp); err != nil {
		t.Fatalf("decode generated MCP configuration: %v", err)
	}
	codegraph := mcp.Servers["codegraph"]
	if codegraph.Type != "stdio" || codegraph.Command != "codegraph" {
		t.Errorf(
			"CodeGraph MCP transport = %q %q",
			codegraph.Type,
			codegraph.Command,
		)
	}
	if strings.Join(codegraph.Args, " ") !=
		"serve --mcp --path ${CLAUDE_PROJECT_DIR}" {
		t.Errorf("CodeGraph MCP args = %v", codegraph.Args)
	}
	if !strings.Contains(
		codegraph.Env["HIGURASHI_GENERATED"],
		"Higurashi-Generated:",
	) {
		t.Error("MCP configuration is missing generated ownership")
	}
}

func TestClaudeRoleContracts(t *testing.T) {
	bundle, err := render.Build("claude-code", "test")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	files := make(map[string]string, len(bundle.Files))
	for _, file := range bundle.Files {
		files[file.Path] = string(file.Source)
	}

	skill := files["skills/deliver/SKILL.md"]
	requireContains(t, skill,
		"name: deliver",
		"disable-model-invocation: true",
		"$ARGUMENTS",
		"higurashi-loop:higurashi-plan",
		"higurashi-loop:higurashi-apply",
		"higurashi config validate --json",
		"higurashi verification suggest --json",
		"`Permitted commands:`",
		"artifact path and hash",
		"allowed scope",
		"relevant instructions",
		"For a legacy task",
		"failing or passing evidence",
		"verification.requiredCommands",
		"Configured commands are command authority",
		"Do not block for a missing task-local command when an applicable configured command covers the required evidence.",
		"Run `higurashi verification suggest --json` only when no configured command covers the missing evidence.",
		"A clear retry reply",
		"restart PRECHECK for the same work-item ID and original runner options",
		"A retry never authorizes a repair round",
		"user-owned",
		"loop.maxApplyBatchesPerRun",
		"loop.maxRepairAttempts",
		"--repair",
		"repair_recovery_required",
		"`repair_plan_required`",
		"invoke PLAN exactly once",
		"repair authorization while repair planning is required",
		"require `repair_ready`",
		"artifact status is `refined`",
		"/higurashi-loop:refine",
	)
	if strings.Contains(skill, "context: fork") {
		t.Error("Claude main skill forks instead of coordinating in context")
	}

	refineSkill := files["skills/refine/SKILL.md"]
	requireContains(t, refineSkill,
		"name: refine",
		"disable-model-invocation: true",
		"$ARGUMENTS",
		"higurashi-loop:higurashi-refine",
		"at most one follow-up",
		"explicit confirmation",
		"Status: refined",
	)

	for _, name := range []string{
		"higurashi-refine.md",
		"higurashi-plan.md",
		"higurashi-apply.md",
		"higurashi-verify-contract.md",
		"higurashi-verify-risk.md",
	} {
		plugin := files["agents/"+name]
		standalone := files[".claude/agents/"+name]
		if plugin == "" || standalone == "" {
			t.Errorf("missing plugin or standalone Claude agent %s", name)
			continue
		}
		if plugin != standalone {
			t.Errorf("plugin and standalone agent sources differ for %s", name)
		}
		requireContains(t, plugin,
			"model: inherit",
			"effort:",
			"maxTurns:",
			"disallowedTools: Agent",
			render.AntiBypassRule,
			"CodeGraph",
		)
		if strings.Contains(plugin, "\nisolation:") {
			t.Errorf("%s configures worktree isolation", name)
		}
	}

	refiner := files["agents/higurashi-refine.md"]
	requireContains(t, refiner,
		"product-owner",
		"questions",
		"draft",
		"persist",
		"Never modify the requirement source",
		"Status: refined",
	)

	apply := files["agents/higurashi-apply.md"]
	requireContains(t, apply,
		"tools: Read, Grep, Glob, Write, Edit, Bash,",
		"exactly one task",
		"Run only commands in that exact list",
		"EnterWorktree",
		"ExitWorktree",
		"Never change reviewer receipts",
		"Never\ninvoke `higurashi repair authorize`",
	)
	planner := files["agents/higurashi-plan.md"]
	requireContains(t, planner,
		"`Permitted commands:`",
		"For `repair_plan_required`",
	)
	for _, name := range []string{
		"agents/higurashi-verify-contract.md",
		"agents/higurashi-verify-risk.md",
	} {
		reviewer := files[name]
		requireContains(t, reviewer,
			"tools: Read, Grep, Glob, mcp__codegraph__*",
			"disallowedTools: Agent, Write, Edit, NotebookEdit, Bash",
			"Remain read-only, never delegate",
			"minimum acceptance condition",
		)
	}
}

func TestBuildRejectsHeaderInjectionThroughGeneratorVersion(t *testing.T) {
	_, err := render.Build("opencode", "1.2.3\nforged=true")

	if err == nil {
		t.Fatal("Build() error = nil, want unsafe version refusal")
	}
}

func TestApplyRefusesLocallyModifiedGeneratedFileWithoutWriting(t *testing.T) {
	root := t.TempDir()
	bundle, err := render.Build("opencode", "1.2.3")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if _, err := render.Apply(root, bundle); err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}

	name := filepath.Join(root, filepath.FromSlash(bundle.Files[0].Path))
	locallyModified := append(
		append([]byte(nil), bundle.Files[0].Content...),
		[]byte("\nlocal edit\n")...,
	)
	if err := os.WriteFile(name, locallyModified, 0o600); err != nil {
		t.Fatalf("write local modification: %v", err)
	}
	beforeManifest, err := os.ReadFile(
		filepath.Join(root, ".higurashi", "generated.json"),
	)
	if err != nil {
		t.Fatalf("read manifest before conflict: %v", err)
	}

	report, err := render.Apply(root, bundle)

	if !errors.Is(err, render.ErrConflict) {
		t.Fatalf("Apply() error = %v, want ErrConflict", err)
	}
	if len(report.Conflicts) != 1 ||
		report.Conflicts[0] != bundle.Files[0].Path {
		t.Errorf("report.Conflicts = %v, want modified path", report.Conflicts)
	}
	actual, readErr := os.ReadFile(name)
	if readErr != nil {
		t.Fatalf("read locally modified file: %v", readErr)
	}
	if string(actual) != string(locallyModified) {
		t.Error("conflicting apply overwrote the local modification")
	}
	afterManifest, err := os.ReadFile(
		filepath.Join(root, ".higurashi", "generated.json"),
	)
	if err != nil {
		t.Fatalf("read manifest after conflict: %v", err)
	}
	if string(afterManifest) != string(beforeManifest) {
		t.Error("conflicting apply changed the manifest")
	}
}

func TestApplyForceGeneratedRepairsRecognizedLocalModification(t *testing.T) {
	root := t.TempDir()
	bundle, err := render.Build("opencode", "1.2.3")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if _, err := render.Apply(root, bundle); err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}
	target := filepath.Join(root, filepath.FromSlash(bundle.Files[0].Path))
	if err := os.WriteFile(
		target,
		append(bundle.Files[0].Content, []byte("\nrecognized local edit\n")...),
		0o600,
	); err != nil {
		t.Fatalf("modify generated target: %v", err)
	}

	report, err := render.ApplyWithOptions(
		root,
		bundle,
		render.ApplyOptions{ForceGenerated: true},
	)

	if err != nil {
		t.Fatalf("ApplyWithOptions() error = %v", err)
	}
	if len(report.Conflicts) != 0 ||
		!slices.Contains(report.ChangedPaths, bundle.Files[0].Path) {
		t.Errorf("report = %+v, want forced update", report)
	}
	actual, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read forced target: %v", err)
	}
	if !bytes.Equal(actual, bundle.Files[0].Content) {
		t.Error("forced apply did not restore canonical content")
	}
}

func TestApplyIsIdempotentWhenGeneratedFilesAreCurrent(t *testing.T) {
	root := t.TempDir()
	bundle, err := render.Build("claude-code", "1.2.3")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if _, err := render.Apply(root, bundle); err != nil {
		t.Fatalf("first Apply() error = %v", err)
	}
	skillName := filepath.Join(
		root,
		filepath.FromSlash(bundle.Files[0].Path),
	)
	manifestName := filepath.Join(root, ".higurashi", "generated.json")
	skillBefore, err := os.Stat(skillName)
	if err != nil {
		t.Fatalf("stat skill before second apply: %v", err)
	}
	manifestBefore, err := os.Stat(manifestName)
	if err != nil {
		t.Fatalf("stat manifest before second apply: %v", err)
	}

	report, err := render.Apply(root, bundle)

	if err != nil {
		t.Fatalf("second Apply() error = %v", err)
	}
	if len(report.ChangedPaths) != 0 {
		t.Errorf("report.ChangedPaths = %v, want empty", report.ChangedPaths)
	}
	skillAfter, err := os.Stat(skillName)
	if err != nil {
		t.Fatalf("stat skill after second apply: %v", err)
	}
	manifestAfter, err := os.Stat(manifestName)
	if err != nil {
		t.Fatalf("stat manifest after second apply: %v", err)
	}
	if !os.SameFile(skillBefore, skillAfter) {
		t.Error("second apply replaced the current generated skill")
	}
	if !os.SameFile(manifestBefore, manifestAfter) {
		t.Error("second apply replaced the unchanged manifest")
	}
}

func TestApplyRefusesUnrecognizedExistingFile(t *testing.T) {
	root := t.TempDir()
	bundle, err := render.Build("opencode", "1.2.3")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	name := filepath.Join(root, filepath.FromSlash(bundle.Files[0].Path))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatalf("create target directory: %v", err)
	}
	original := []byte("user-owned file\n")
	if err := os.WriteFile(name, original, 0o600); err != nil {
		t.Fatalf("write user-owned file: %v", err)
	}

	report, err := render.Apply(root, bundle)

	if !errors.Is(err, render.ErrConflict) {
		t.Fatalf("Apply() error = %v, want ErrConflict", err)
	}
	if len(report.Conflicts) != 1 ||
		report.Conflicts[0] != bundle.Files[0].Path {
		t.Errorf("report.Conflicts = %v, want existing path", report.Conflicts)
	}
	actual, readErr := os.ReadFile(name)
	if readErr != nil {
		t.Fatalf("read existing file: %v", readErr)
	}
	if string(actual) != string(original) {
		t.Error("apply overwrote the unrecognized existing file")
	}
}

func TestApplyRefusesGeneratedPathResolvingIntoGitMetadata(t *testing.T) {
	root := t.TempDir()
	gitDirectory := filepath.Join(root, ".git")
	if err := os.MkdirAll(gitDirectory, 0o755); err != nil {
		t.Fatalf("create Git metadata directory: %v", err)
	}
	if err := os.Symlink(gitDirectory, filepath.Join(root, ".agents")); err != nil {
		t.Skipf("create adapter symlink: %v", err)
	}
	bundle, err := render.Build("opencode", "1.2.3")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	_, err = render.Apply(root, bundle)

	if err == nil {
		t.Fatal("Apply() error = nil, want unsafe mutation refusal")
	}
	if _, statErr := os.Stat(filepath.Join(gitDirectory, "skills")); !errors.Is(
		statErr,
		os.ErrNotExist,
	) {
		t.Errorf("Git metadata was modified, stat error = %v", statErr)
	}
}

func TestAgentTemplateMustRepeatAntiBypassRule(t *testing.T) {
	root := t.TempDir()
	bundle := render.Bundle{
		Adapter:          "opencode",
		GeneratorVersion: "test",
		Files: []render.File{{
			Path:    ".opencode/agents/higurashi-apply.md",
			Content: []byte("You may implement the assigned task."),
		}},
	}

	_, err := render.Apply(root, bundle)

	if !errors.Is(err, render.ErrMissingAntiBypassRule) {
		t.Fatalf(
			"Apply() error = %v, want ErrMissingAntiBypassRule",
			err,
		)
	}
	bundle.Files[0].Content = []byte(render.AntiBypassRule)
	if err := render.ValidateBundle(bundle); err != nil {
		t.Fatalf("ValidateBundle() with rule error = %v", err)
	}
}

func requireContains(t *testing.T, content string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(content, value) {
			t.Errorf("content is missing %q", value)
		}
	}
}
