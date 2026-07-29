package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/render"
	"github.com/jpmartinez/higurashi-loop/internal/result"
)

const modelCommandOutputLimit = 1 << 20

type modelsArguments struct {
	Operation string
	Runner    string
	JSON      bool
	Help      bool
	Changes   map[string]string
	Efforts   map[string]string
}

type modelEntry struct {
	Role    string `json:"role"`
	Model   string `json:"model"`
	Variant string `json:"variant,omitempty"`
}

func runModels(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	arguments, err := parseModelsArguments(args)
	if arguments.Help {
		writeModelsHelp(stdout)
		return exitSuccess
	}
	command := "models"
	if arguments.Operation != "" {
		command += "." + arguments.Operation
	}
	if err != nil {
		return writeUsageFailure(
			stdout,
			stderr,
			arguments.JSON,
			command,
			err.Error(),
		)
	}

	root, err := project.ResolveRoot(
		ctx,
		options.WorkingDirectory,
		options.CommandRunner,
	)
	if err != nil {
		kind := "not_git_project"
		if errors.Is(err, project.ErrUnsafeProjectRoot) {
			kind = "unsafe_project_root"
		}
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			command,
			kind,
			err.Error(),
			exitUnsafeProject,
		)
	}

	configuration, err := config.Load(root)
	if err != nil {
		kind := "configuration_invalid"
		if errors.Is(err, config.ErrMissing) {
			kind = "configuration_missing"
		}
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			command,
			kind,
			err.Error(),
			exitInvalidConfiguration,
		)
	}
	runner, ok := configuration.Runner(arguments.Runner)
	if !ok {
		return writeUsageFailure(
			stdout,
			stderr,
			arguments.JSON,
			command,
			fmt.Sprintf("unsupported runner %q", arguments.Runner),
		)
	}
	if !runner.Enabled {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			command,
			"runner_disabled",
			fmt.Sprintf("runner %q is disabled", arguments.Runner),
			exitInvalidConfiguration,
		)
	}

	switch arguments.Operation {
	case "show":
		return writeModelsResult(
			stdout,
			stderr,
			arguments.JSON,
			command,
			"configured",
			root,
			arguments.Runner,
			runner.Models,
			nil,
			exitSuccess,
			"",
		)
	case "validate":
		return validateRunnerModels(
			ctx,
			stdout,
			stderr,
			arguments,
			options,
			root,
			runner.Models,
		)
	case "set":
		return setRunnerModels(
			stdout,
			stderr,
			arguments,
			options,
			root,
			configuration,
		)
	default:
		panic("unreachable models operation")
	}
}

func setRunnerModels(
	stdout io.Writer,
	stderr io.Writer,
	arguments modelsArguments,
	options Options,
	root string,
	configuration config.Config,
) int {
	if arguments.Runner != "opencode" {
		return writeUsageFailure(
			stdout,
			stderr,
			arguments.JSON,
			"models.set",
			"model rendering is currently supported only for opencode",
		)
	}

	assignments := configuration.Runners.OpenCode.Models
	for role, model := range arguments.Changes {
		setModelForRole(&assignments, role, model)
	}
	for role, effort := range arguments.Efforts {
		reference := modelForRole(assignments, role)
		updated, err := config.WithModelVariant(reference, effort)
		if err != nil {
			return writeFailure(
				stdout,
				stderr,
				arguments.JSON,
				"models.set",
				"models_invalid",
				fmt.Sprintf("%s effort: %v", role, err),
				exitInvalidConfiguration,
			)
		}
		setModelForRole(&assignments, role, updated)
	}
	if err := config.ValidateModelAssignments(assignments); err != nil {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"models.set",
			"models_invalid",
			err.Error(),
			exitInvalidConfiguration,
		)
	}

	bundle, err := render.BuildConfigured(
		arguments.Runner,
		options.Version,
		assignments,
	)
	if err != nil {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"models.set",
			"render_failed",
			err.Error(),
			exitInternalError,
		)
	}
	report, err := render.PreflightApply(root, bundle, render.ApplyOptions{})
	if err != nil {
		kind := "render_failed"
		code := exitInternalError
		if errors.Is(err, render.ErrConflict) {
			kind = "generated_file_conflict"
			code = exitUnsafeProject
		}
		return writeModelsResult(
			stdout,
			stderr,
			arguments.JSON,
			"models.set",
			kind,
			root,
			arguments.Runner,
			assignments,
			report.Conflicts,
			code,
			err.Error(),
		)
	}

	configuration.Runners.OpenCode.Models = assignments
	encoded, err := config.Encode(configuration)
	if err != nil {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"models.set",
			"configuration_invalid",
			err.Error(),
			exitInvalidConfiguration,
		)
	}
	if err := replaceProjectConfig(root, encoded); err != nil {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"models.set",
			"configuration_write_failed",
			err.Error(),
			exitInternalError,
		)
	}

	report, err = render.Apply(root, bundle)
	if err != nil {
		return writeModelsResult(
			stdout,
			stderr,
			arguments.JSON,
			"models.set",
			"partial_update",
			root,
			arguments.Runner,
			assignments,
			report.Conflicts,
			exitConflict,
			"configuration was updated, but adapter regeneration failed; rerun higurashi adapter update opencode",
		)
	}
	changed := append(
		[]string{".higurashi/config.json"},
		report.ChangedPaths...,
	)
	sort.Strings(changed)
	return writeModelsResult(
		stdout,
		stderr,
		arguments.JSON,
		"models.set",
		"updated",
		root,
		arguments.Runner,
		assignments,
		changed,
		exitSuccess,
		"",
	)
}

func validateRunnerModels(
	ctx context.Context,
	stdout io.Writer,
	stderr io.Writer,
	arguments modelsArguments,
	options Options,
	root string,
	assignments config.ModelAssignments,
) int {
	if arguments.Runner != "opencode" {
		return writeUsageFailure(
			stdout,
			stderr,
			arguments.JSON,
			"models.validate",
			"availability validation is currently supported only for opencode",
		)
	}
	output, err := options.CommandRunner.Output(
		ctx,
		modelCommandOutputLimit,
		"opencode",
		"models",
		"--verbose",
	)
	if err != nil {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"models.validate",
			"runner_unavailable",
			fmt.Sprintf("list OpenCode models: %v", err),
			exitCapabilityUnavailable,
		)
	}
	var missing []string
	for _, entry := range modelEntries(assignments) {
		if entry.Model == "inherit" {
			continue
		}
		model, variant := config.SplitModelReference(entry.Model)
		metadata, found, metadataErr := openCodeModelMetadata(output, model)
		if metadataErr != nil {
			return writeFailure(
				stdout,
				stderr,
				arguments.JSON,
				"models.validate",
				"catalog_invalid",
				fmt.Sprintf("parse OpenCode model metadata for %s: %v", model, metadataErr),
				exitCapabilityUnavailable,
			)
		}
		if !found {
			missing = append(
				missing,
				fmt.Sprintf("%s=%s", entry.Role, entry.Model),
			)
			continue
		}
		if variant != "" {
			if _, ok := metadata.Variants[variant]; !ok {
				missing = append(
					missing,
					fmt.Sprintf(
						"%s=%s (variant unavailable)",
						entry.Role,
						entry.Model,
					),
				)
			}
		}
	}
	if len(missing) != 0 {
		return writeModelsResult(
			stdout,
			stderr,
			arguments.JSON,
			"models.validate",
			"models_unavailable",
			root,
			arguments.Runner,
			assignments,
			missing,
			exitCapabilityUnavailable,
			"configured models or variants are absent from the OpenCode model catalog",
		)
	}
	return writeModelsResult(
		stdout,
		stderr,
		arguments.JSON,
		"models.validate",
		"valid",
		root,
		arguments.Runner,
		assignments,
		nil,
		exitSuccess,
		"",
	)
}

func parseModelsArguments(args []string) (modelsArguments, error) {
	parsed := modelsArguments{
		Runner:  "opencode",
		Changes: make(map[string]string),
		Efforts: make(map[string]string),
	}
	if len(args) == 0 {
		return parsed, errors.New("models requires show, set, or validate")
	}
	if args[0] == "-h" || args[0] == "--help" {
		parsed.Help = true
		return parsed, nil
	}
	parsed.Operation = args[0]
	switch parsed.Operation {
	case "show", "set", "validate":
	default:
		return parsed, fmt.Errorf(
			"unsupported models operation %q",
			parsed.Operation,
		)
	}

	roleFlags := map[string]string{
		"--orchestrator":    "orchestrator",
		"--refine":          "refine",
		"--plan":            "plan",
		"--apply":           "apply",
		"--verify-contract": "verify-contract",
		"--verify-risk":     "verify-risk",
	}
	effortFlags := map[string]string{
		"--orchestrator-effort":    "orchestrator",
		"--refine-effort":          "refine",
		"--plan-effort":            "plan",
		"--apply-effort":           "apply",
		"--verify-contract-effort": "verify-contract",
		"--verify-risk-effort":     "verify-risk",
	}
	for index := 1; index < len(args); index++ {
		argument := args[index]
		switch argument {
		case "--json":
			if parsed.JSON {
				return parsed, errors.New("--json may be supplied only once")
			}
			parsed.JSON = true
		case "-h", "--help":
			parsed.Help = true
			return parsed, nil
		case "--runner":
			index++
			if index >= len(args) {
				return parsed, errors.New("--runner requires a value")
			}
			parsed.Runner = args[index]
		default:
			role, ok := roleFlags[argument]
			if !ok {
				role, ok = effortFlags[argument]
				if !ok {
					return parsed, fmt.Errorf("unknown option %q", argument)
				}
				if parsed.Operation != "set" {
					return parsed, fmt.Errorf(
						"%s is valid only with models set",
						argument,
					)
				}
				if _, duplicate := parsed.Efforts[role]; duplicate {
					return parsed, fmt.Errorf(
						"%s may be supplied only once",
						argument,
					)
				}
				index++
				if index >= len(args) {
					return parsed, fmt.Errorf("%s requires a value", argument)
				}
				parsed.Efforts[role] = args[index]
				continue
			}
			if parsed.Operation != "set" {
				return parsed, fmt.Errorf(
					"%s is valid only with models set",
					argument,
				)
			}
			if _, duplicate := parsed.Changes[role]; duplicate {
				return parsed, fmt.Errorf("%s may be supplied only once", argument)
			}
			index++
			if index >= len(args) {
				return parsed, fmt.Errorf("%s requires a value", argument)
			}
			parsed.Changes[role] = args[index]
		}
	}
	switch parsed.Runner {
	case "opencode", "claude-code":
	default:
		return parsed, fmt.Errorf("unsupported runner %q", parsed.Runner)
	}
	if parsed.Operation == "set" &&
		len(parsed.Changes) == 0 &&
		len(parsed.Efforts) == 0 {
		return parsed, errors.New("models set requires at least one role assignment")
	}
	return parsed, nil
}

func setModelForRole(
	assignments *config.ModelAssignments,
	role, model string,
) {
	switch role {
	case "orchestrator":
		assignments.Orchestrator = model
	case "refine":
		assignments.Refine = model
	case "plan":
		assignments.Plan = model
	case "apply":
		assignments.Apply = model
	case "verify-contract":
		assignments.VerifyContract = model
	case "verify-risk":
		assignments.VerifyRisk = model
	}
}

func modelForRole(assignments config.ModelAssignments, role string) string {
	switch role {
	case "orchestrator":
		return assignments.Orchestrator
	case "refine":
		return assignments.Refine
	case "plan":
		return assignments.Plan
	case "apply":
		return assignments.Apply
	case "verify-contract":
		return assignments.VerifyContract
	case "verify-risk":
		return assignments.VerifyRisk
	default:
		return "inherit"
	}
}

func modelEntries(assignments config.ModelAssignments) []modelEntry {
	entries := []modelEntry{
		{Role: "orchestrator", Model: assignments.Orchestrator},
		{Role: "refine", Model: assignments.Refine},
		{Role: "plan", Model: assignments.Plan},
		{Role: "apply", Model: assignments.Apply},
		{Role: "verify-contract", Model: assignments.VerifyContract},
		{Role: "verify-risk", Model: assignments.VerifyRisk},
	}
	for index := range entries {
		_, entries[index].Variant = config.SplitModelReference(
			entries[index].Model,
		)
	}
	return entries
}

type openCodeModel struct {
	Variants map[string]json.RawMessage `json:"variants"`
}

func openCodeModelMetadata(
	output []byte,
	model string,
) (openCodeModel, bool, error) {
	output = bytes.ReplaceAll(output, []byte("\r\n"), []byte("\n"))
	marker := []byte(model + "\n")
	searchFrom := 0
	for {
		index := bytes.Index(output[searchFrom:], marker)
		if index < 0 {
			return openCodeModel{}, false, nil
		}
		index += searchFrom
		if index == 0 || output[index-1] == '\n' {
			remainder := output[index+len(marker):]
			remainder = bytes.TrimLeft(remainder, " \t\n")
			if len(remainder) == 0 || remainder[0] != '{' {
				return openCodeModel{}, true, nil
			}
			var metadata openCodeModel
			if err := json.NewDecoder(bytes.NewReader(remainder)).Decode(
				&metadata,
			); err != nil {
				return openCodeModel{}, true, err
			}
			return metadata, true, nil
		}
		searchFrom = index + len(marker)
	}
}

func replaceProjectConfig(root string, content []byte) error {
	name, err := project.ResolveContainedPath(root, ".higurashi/config.json")
	if err != nil {
		return err
	}
	if err := project.ValidateMutationPath(root, name); err != nil {
		return err
	}
	info, err := os.Stat(name)
	if err != nil {
		return fmt.Errorf("stat configuration: %w", err)
	}
	temporary, err := os.CreateTemp(
		filepath.Dir(name),
		"."+filepath.Base(name)+".tmp-*",
	)
	if err != nil {
		return fmt.Errorf("create temporary configuration: %w", err)
	}
	temporaryName := temporary.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(temporaryName)
		}
	}()
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary configuration: %w", err)
	}
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set configuration permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary configuration: %w", err)
	}
	if err := os.Rename(temporaryName, name); err != nil {
		return fmt.Errorf("replace configuration: %w", err)
	}
	cleanup = false
	return nil
}

func writeModelsResult(
	stdout io.Writer,
	stderr io.Writer,
	jsonOutput bool,
	command string,
	kind string,
	root string,
	runner string,
	assignments config.ModelAssignments,
	details []string,
	exitCode int,
	message string,
) int {
	ok := exitCode == exitSuccess
	if jsonOutput {
		envelope := result.Envelope{
			Command:      command,
			OK:           ok,
			Kind:         kind,
			Message:      message,
			ProjectRoot:  root,
			Runner:       runner,
			Models:       modelEntries(assignments),
			ChangedPaths: details,
		}
		if err := result.WriteJSON(stdout, envelope); err != nil {
			fmt.Fprintf(stderr, "higurashi: write JSON result: %v\n", err)
			return exitInternalError
		}
	} else {
		switch kind {
		case "configured":
			fmt.Fprintln(stdout, "Higurashi model assignments")
		case "valid":
			fmt.Fprintln(stdout, "Higurashi model assignments: valid")
		case "updated":
			fmt.Fprintln(stdout, "Higurashi model assignments: updated")
		default:
			fmt.Fprintf(stdout, "Higurashi model assignments: %s\n", kind)
		}
		fmt.Fprintf(stdout, "Project root: %s\n", root)
		fmt.Fprintf(stdout, "Runner: %s\n", runner)
		for _, entry := range modelEntries(assignments) {
			fmt.Fprintf(stdout, "%s: %s\n", entry.Role, entry.Model)
		}
		for _, detail := range details {
			fmt.Fprintf(stdout, "Detail: %s\n", detail)
		}
		if !ok && message != "" {
			fmt.Fprintf(stderr, "higurashi: %s\n", message)
		}
	}
	return exitCode
}

func writeModelsHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  higurashi models show [--runner opencode] [--json]
  higurashi models validate [--runner opencode] [--json]
  higurashi models set [--runner opencode] [ROLE OPTIONS] [--json]

Role options:
  --orchestrator MODEL
  --orchestrator-effort EFFORT
  --refine MODEL
  --refine-effort EFFORT
  --plan MODEL
  --plan-effort EFFORT
  --apply MODEL
  --apply-effort EFFORT
  --verify-contract MODEL
  --verify-contract-effort EFFORT
  --verify-risk MODEL
  --verify-risk-effort EFFORT

MODEL is "inherit" or an exact provider/model ID. EFFORT is a model-specific
OpenCode variant such as low, high, or max. Use "inherit" to clear an effort.
Exact model and variant availability is checked by:
  higurashi models validate

models set preserves unspecified roles, atomically updates
.higurashi/config.json, and regenerates Higurashi-owned OpenCode agents.`)
}
