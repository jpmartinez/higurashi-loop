package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jpmartinez/higurashi-loop/internal/cli"
)

func TestAdapterInstallAndDiffCanonicalProtocol(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "preferred")

	exitCode, installed, stderr := runAdapterJSON(
		t,
		repository,
		"install",
		"opencode",
	)

	if exitCode != 0 {
		t.Fatalf(
			"install exit code = %d, want 0\nstderr:\n%s",
			exitCode,
			stderr,
		)
	}
	if installed.Kind != "installed" {
		t.Errorf("install kind = %q, want %q", installed.Kind, "installed")
	}
	if len(installed.ChangedPaths) != 13 {
		t.Errorf(
			"changedPaths = %v, want twelve OpenCode files and manifest",
			installed.ChangedPaths,
		)
	}
	skillName := filepath.Join(
		repository,
		".agents",
		"skills",
		"higurashi-deliver",
		"SKILL.md",
	)
	skill, err := os.ReadFile(skillName)
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}
	if !bytes.Contains(skill, []byte("Higurashi-Generated:")) {
		t.Error("installed skill is missing generated ownership header")
	}
	for _, relative := range []string{
		".agents/skills/higurashi-refine/SKILL.md",
		".opencode/commands/higurashi-deliver.md",
		".opencode/commands/higurashi-refine.md",
		".opencode/agents/higurashi-orchestrator.md",
		".opencode/agents/higurashi-refine.md",
		".opencode/agents/higurashi-plan.md",
		".opencode/agents/higurashi-apply.md",
		".opencode/agents/higurashi-verify-contract.md",
		".opencode/agents/higurashi-verify-risk.md",
	} {
		content, readErr := os.ReadFile(filepath.Join(
			repository,
			filepath.FromSlash(relative),
		))
		if readErr != nil {
			t.Errorf("read installed OpenCode file %s: %v", relative, readErr)
			continue
		}
		if !bytes.Contains(content, []byte("Higurashi-Generated:")) {
			t.Errorf("%s is missing generated ownership header", relative)
		}
	}

	manifestName := filepath.Join(repository, ".higurashi", "generated.json")
	before, err := os.Stat(manifestName)
	if err != nil {
		t.Fatalf("stat manifest before diff: %v", err)
	}
	exitCode, diff, stderr := runAdapterJSON(
		t,
		repository,
		"diff",
		"opencode",
	)
	if exitCode != 0 {
		t.Fatalf(
			"diff exit code = %d, want 0\nstderr:\n%s",
			exitCode,
			stderr,
		)
	}
	if diff.Kind != "clean" {
		t.Errorf("diff kind = %q, want %q", diff.Kind, "clean")
	}
	after, err := os.Stat(manifestName)
	if err != nil {
		t.Fatalf("stat manifest after diff: %v", err)
	}
	if !os.SameFile(before, after) {
		t.Error("adapter diff replaced the generated manifest")
	}
}

func TestAdapterUpdateRefusesLocalModification(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "preferred")
	if exitCode, _, _ := runAdapterJSON(
		t,
		repository,
		"install",
		"claude-code",
	); exitCode != 0 {
		t.Fatalf("install exit code = %d, want 0", exitCode)
	}
	skillName := filepath.Join(
		repository,
		".claude",
		"skills",
		"higurashi-deliver",
		"SKILL.md",
	)
	file, err := os.OpenFile(skillName, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open generated skill: %v", err)
	}
	if _, err := file.WriteString("\nlocal modification\n"); err != nil {
		_ = file.Close()
		t.Fatalf("modify generated skill: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close generated skill: %v", err)
	}
	before, err := os.ReadFile(skillName)
	if err != nil {
		t.Fatalf("read modified skill: %v", err)
	}

	exitCode, result, _ := runAdapterJSON(
		t,
		repository,
		"update",
		"claude-code",
	)

	if exitCode != 8 {
		t.Errorf("update exit code = %d, want 8", exitCode)
	}
	if result.Kind != "generated_file_conflict" {
		t.Errorf(
			"kind = %q, want %q",
			result.Kind,
			"generated_file_conflict",
		)
	}
	if len(result.Conflicts) != 1 {
		t.Errorf("conflicts = %v, want modified skill", result.Conflicts)
	}
	after, err := os.ReadFile(skillName)
	if err != nil {
		t.Fatalf("read skill after refused update: %v", err)
	}
	if string(after) != string(before) {
		t.Error("refused update overwrote the local modification")
	}
}

func TestClaudeFixtureInstallsPluginAndStandaloneVariant(t *testing.T) {
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "preferred")

	exitCode, installed, stderr := runAdapterJSON(
		t,
		repository,
		"install",
		"claude-code",
	)

	if exitCode != 0 {
		t.Fatalf(
			"install exit code = %d, want 0\nstderr:\n%s",
			exitCode,
			stderr,
		)
	}
	if installed.Kind != "installed" {
		t.Errorf("install kind = %q, want installed", installed.Kind)
	}
	if len(installed.ChangedPaths) != 19 {
		t.Errorf(
			"changedPaths = %v, want eighteen Claude files and manifest",
			installed.ChangedPaths,
		)
	}
	for _, relative := range []string{
		".claude-plugin/plugin.json",
		".mcp.json",
		"skills/deliver/SKILL.md",
		"skills/refine/SKILL.md",
		"agents/higurashi-refine.md",
		"agents/higurashi-plan.md",
		"agents/higurashi-apply.md",
		"agents/higurashi-verify-contract.md",
		"agents/higurashi-verify-risk.md",
		".claude/skills/higurashi-deliver/SKILL.md",
		".claude/skills/higurashi-refine/SKILL.md",
		".claude/agents/higurashi-refine.md",
		".claude/agents/higurashi-plan.md",
		".claude/agents/higurashi-apply.md",
		".claude/agents/higurashi-verify-contract.md",
		".claude/agents/higurashi-verify-risk.md",
	} {
		content, err := os.ReadFile(filepath.Join(
			repository,
			filepath.FromSlash(relative),
		))
		if err != nil {
			t.Errorf("read installed Claude file %s: %v", relative, err)
			continue
		}
		if !bytes.Contains(content, []byte("Higurashi-Generated:")) {
			t.Errorf("%s is missing generated ownership", relative)
		}
	}
	for _, relative := range []string{
		".claude-plugin/plugin.json",
		".mcp.json",
	} {
		content, err := os.ReadFile(filepath.Join(
			repository,
			filepath.FromSlash(relative),
		))
		if err != nil {
			t.Fatalf("read installed JSON %s: %v", relative, err)
		}
		var value any
		if err := json.Unmarshal(content, &value); err != nil {
			t.Errorf("installed JSON %s is invalid: %v", relative, err)
		}
	}
}

func TestClaudeFixtureValidatesWhenCLIIsAvailable(t *testing.T) {
	claude, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude is not installed")
	}
	help := exec.Command(claude, "plugin", "validate", "--help")
	if output, helpErr := help.CombinedOutput(); helpErr != nil {
		t.Skipf("installed Claude CLI has no plugin validator: %v\n%s", helpErr, output)
	}

	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "preferred")
	if exitCode, _, stderr := runAdapterJSON(
		t,
		repository,
		"install",
		"claude-code",
	); exitCode != 0 {
		t.Fatalf(
			"install exit code = %d, want 0\nstderr:\n%s",
			exitCode,
			stderr,
		)
	}

	command := exec.Command(
		claude,
		"plugin",
		"validate",
		repository,
		"--strict",
	)
	command.Dir = repository
	command.Env = append(
		os.Environ(),
		"CLAUDE_CONFIG_DIR="+t.TempDir(),
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("claude plugin validate: %v\n%s", err, output)
	}
}

func TestOpenCodeFixtureIsDiscoverable(t *testing.T) {
	opencode, err := exec.LookPath("opencode")
	if err != nil {
		t.Skip("opencode is not installed")
	}
	repository := newGitRepository(t)
	writeMinimalConfig(t, repository, "preferred")
	if exitCode, _, stderr := runAdapterJSON(
		t,
		repository,
		"install",
		"opencode",
	); exitCode != 0 {
		t.Fatalf(
			"install exit code = %d, want 0\nstderr:\n%s",
			exitCode,
			stderr,
		)
	}

	command := exec.Command(opencode, "debug", "config")
	command.Dir = repository
	runtimeHome := t.TempDir()
	command.Env = append(
		os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(runtimeHome, "config"),
		"XDG_DATA_HOME="+filepath.Join(runtimeHome, "data"),
		"XDG_CACHE_HOME="+filepath.Join(runtimeHome, "cache"),
		"XDG_STATE_HOME="+filepath.Join(runtimeHome, "state"),
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("opencode debug config: %v\n%s", err, output)
	}
	var effective struct {
		Agents   map[string]json.RawMessage `json:"agent"`
		Commands map[string]json.RawMessage `json:"command"`
	}
	if err := json.Unmarshal(output, &effective); err != nil {
		t.Fatalf("decode OpenCode effective config: %v", err)
	}
	for _, name := range []string{
		"higurashi-orchestrator",
		"higurashi-plan",
		"higurashi-apply",
		"higurashi-verify-contract",
		"higurashi-verify-risk",
	} {
		if _, exists := effective.Agents[name]; !exists {
			t.Errorf("OpenCode did not discover agent %q", name)
		}
	}
	var deliveryCommand struct {
		Agent   string `json:"agent"`
		Subtask bool   `json:"subtask"`
	}
	rawCommand, exists := effective.Commands["higurashi-deliver"]
	if !exists {
		t.Error("OpenCode did not discover command \"higurashi-deliver\"")
	} else if err := json.Unmarshal(rawCommand, &deliveryCommand); err != nil {
		t.Fatalf("decode discovered command: %v", err)
	} else {
		if deliveryCommand.Agent != "higurashi-orchestrator" {
			t.Errorf(
				"command agent = %q, want higurashi-orchestrator",
				deliveryCommand.Agent,
			)
		}
		if deliveryCommand.Subtask {
			t.Error("command unexpectedly runs the coordinator as a subtask")
		}
	}
	for _, name := range []string{
		"higurashi-verify-contract",
		"higurashi-verify-risk",
	} {
		assertEffectiveAgentPermission(
			t,
			effective.Agents[name],
			name,
			"subagent",
			map[string]string{
				"edit": "deny",
				"bash": "deny",
				"task": "deny",
			},
		)
	}
}

func assertEffectiveAgentPermission(
	t *testing.T,
	raw json.RawMessage,
	name string,
	wantMode string,
	wantPermissions map[string]string,
) {
	t.Helper()
	var agent struct {
		Mode       string                     `json:"mode"`
		Permission map[string]json.RawMessage `json:"permission"`
	}
	if err := json.Unmarshal(raw, &agent); err != nil {
		t.Fatalf("decode effective agent %s: %v", name, err)
	}
	if agent.Mode != wantMode {
		t.Errorf("agent %s mode = %q, want %q", name, agent.Mode, wantMode)
	}
	for permission, want := range wantPermissions {
		var actual string
		if err := json.Unmarshal(agent.Permission[permission], &actual); err != nil {
			t.Errorf(
				"decode agent %s permission %s: %v",
				name,
				permission,
				err,
			)
			continue
		}
		if actual != want {
			t.Errorf(
				"agent %s permission %s = %q, want %q",
				name,
				permission,
				actual,
				want,
			)
		}
	}
}

type adapterJSONResult struct {
	SchemaVersion int      `json:"schemaVersion"`
	Command       string   `json:"command"`
	OK            bool     `json:"ok"`
	Kind          string   `json:"kind"`
	Message       string   `json:"message"`
	Adapter       string   `json:"adapter"`
	ChangedPaths  []string `json:"changedPaths"`
	Conflicts     []string `json:"conflicts"`
	StalePaths    []string `json:"stalePaths"`
	Files         []struct {
		Path   string `json:"path"`
		Status string `json:"status"`
	} `json:"files"`
}

func runAdapterJSON(
	t *testing.T,
	workingDirectory string,
	operation string,
	adapter string,
) (int, adapterJSONResult, string) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := cli.Run(
		context.Background(),
		[]string{"adapter", operation, adapter, "--json"},
		&stdout,
		&stderr,
		cli.Options{
			WorkingDirectory: workingDirectory,
			Version:          "test",
			CommandRunner: &doctorRunner{
				projectRoot: workingDirectory,
			},
		},
	)

	var result adapterJSONResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf(
			"decode JSON result: %v\nstdout:\n%s\nstderr:\n%s",
			err,
			stdout.String(),
			stderr.String(),
		)
	}
	return exitCode, result, stderr.String()
}
