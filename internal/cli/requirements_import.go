package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/result"
	"github.com/jpmartinez/higurashi-loop/internal/workitem"
)

const maxRequirementImportBytes = 4 << 20

type requirementsImportArguments struct {
	WorkItemID string
	SourcePath string
	Stdin      bool
	JSON       bool
	Help       bool
}

func runRequirements(
	ctx context.Context,
	args []string,
	stdout io.Writer,
	stderr io.Writer,
	options Options,
) int {
	arguments, err := parseRequirementsImportArguments(args)
	if arguments.Help {
		writeRequirementsHelp(stdout)
		return exitSuccess
	}
	if err != nil {
		return writeUsageFailure(
			stdout,
			stderr,
			arguments.JSON,
			"requirements.import",
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
			"requirements.import",
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
			"requirements.import",
			kind,
			err.Error(),
			exitInvalidConfiguration,
		)
	}
	if err := workitem.ValidateID(
		configuration.WorkItems.IDPattern,
		arguments.WorkItemID,
	); err != nil {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"requirements.import",
			"unknown",
			err.Error(),
			exitWorkItemUnavailable,
		)
	}

	sourceKind := "inline"
	sourceLabel := "stdin"
	var sourceContent []byte
	if arguments.SourcePath != "" {
		sourceKind = "file"
		sourceLabel = config.NormalizeProjectPath(arguments.SourcePath)
		sourceContent, err = readRequirementSource(root, sourceLabel)
	} else {
		sourceContent, err = readRequirementInput(options.Input)
	}
	if err != nil {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"requirements.import",
			"requirement_invalid",
			err.Error(),
			exitInvalidConfiguration,
		)
	}
	if len(bytes.TrimSpace(sourceContent)) == 0 {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"requirements.import",
			"requirement_invalid",
			"requirement input is empty",
			exitInvalidConfiguration,
		)
	}
	if !utf8.Valid(sourceContent) {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"requirements.import",
			"requirement_invalid",
			"requirement input must be valid UTF-8 Markdown",
			exitInvalidConfiguration,
		)
	}
	if bytes.IndexByte(sourceContent, 0) >= 0 {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"requirements.import",
			"requirement_invalid",
			"requirement input must not contain a NUL byte",
			exitInvalidConfiguration,
		)
	}

	requirementContent, err := workitem.Extract(
		sourceContent,
		arguments.WorkItemID,
	)
	if err != nil {
		if sourceKind == "inline" && errors.Is(err, workitem.ErrUnknown) {
			requirementContent = makeInlineRequirement(
				arguments.WorkItemID,
				sourceContent,
			)
		} else {
			kind := "requirement_invalid"
			code := exitInvalidConfiguration
			if errors.Is(err, workitem.ErrUnknown) {
				kind = "unknown"
				code = exitWorkItemUnavailable
			} else if errors.Is(err, workitem.ErrConflict) {
				kind = "requirement_conflict"
				code = exitConflict
			}
			return writeFailure(
				stdout,
				stderr,
				arguments.JSON,
				"requirements.import",
				kind,
				err.Error(),
				code,
			)
		}
	}

	managedDirectory := path.Join(
		configuration.Artifacts.Directory,
		"requirements",
	)
	requirementPath := path.Join(
		managedDirectory,
		arguments.WorkItemID+".md",
	)
	if sourceKind == "file" && sourceLabel == requirementPath {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"requirements.import",
			"requirement_invalid",
			"source is already the managed requirement path",
			exitInvalidConfiguration,
		)
	}
	sourceHash := fmt.Sprintf("%x", sha256.Sum256(sourceContent))
	importedContent := importedRequirementDocument(
		arguments.WorkItemID,
		sourceKind,
		sourceLabel,
		sourceHash,
		requirementContent,
	)

	nextConfiguration := configuration
	nextConfiguration.WorkItems.RequirementSources = []string{managedDirectory}
	encodedConfiguration, err := config.Encode(nextConfiguration)
	if err != nil {
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"requirements.import",
			"configuration_invalid",
			err.Error(),
			exitInvalidConfiguration,
		)
	}

	created, err := createRequirementSnapshot(
		root,
		requirementPath,
		importedContent,
	)
	if err != nil {
		kind := "requirement_write_failed"
		code := exitInternalError
		if errors.Is(err, errRequirementConflict) {
			kind = "requirement_conflict"
			code = exitConflict
		}
		if errors.Is(err, project.ErrPathEscapesRoot) ||
			errors.Is(err, project.ErrUnsafeMutationPath) {
			kind = "unsafe_project_root"
			code = exitUnsafeProject
		}
		return writeFailure(
			stdout,
			stderr,
			arguments.JSON,
			"requirements.import",
			kind,
			err.Error(),
			code,
		)
	}

	configurationChanged := !slices.Equal(
		configuration.WorkItems.RequirementSources,
		nextConfiguration.WorkItems.RequirementSources,
	)
	if configurationChanged {
		if err := replaceProjectConfig(root, encodedConfiguration); err != nil {
			return writeFailure(
				stdout,
				stderr,
				arguments.JSON,
				"requirements.import",
				"configuration_update_failed",
				fmt.Sprintf(
					"snapshot is durable at %s; rerun import to recover configuration: %v",
					requirementPath,
					err,
				),
				exitInternalError,
			)
		}
	}

	kind := "current"
	var changedPaths []string
	if created {
		kind = "imported"
		changedPaths = append(changedPaths, requirementPath)
	}
	if configurationChanged {
		kind = "imported"
		changedPaths = append(changedPaths, ".higurashi/config.json")
	}
	slices.Sort(changedPaths)
	envelope := result.Envelope{
		Command:           "requirements.import",
		OK:                true,
		Kind:              kind,
		ProjectRoot:       root,
		WorkItemID:        arguments.WorkItemID,
		RequirementSource: requirementPath,
		RequirementSources: nextConfiguration.WorkItems.
			RequirementSources,
		SourceKind:   sourceKind,
		SourceHash:   sourceHash,
		ChangedPaths: changedPaths,
		NextCommands: []string{
			fmt.Sprintf(
				"higurashi inspect %s --json",
				arguments.WorkItemID,
			),
		},
	}
	return writeRequirementsImportResult(
		stdout,
		stderr,
		arguments.JSON,
		envelope,
	)
}

func parseRequirementsImportArguments(
	args []string,
) (requirementsImportArguments, error) {
	var parsed requirementsImportArguments
	for _, argument := range args {
		if argument == "--json" {
			if parsed.JSON {
				return parsed, errors.New("--json may be supplied only once")
			}
			parsed.JSON = true
		}
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		parsed.Help = true
		return parsed, nil
	}
	if len(args) < 1 || args[0] != "import" {
		return parsed, errors.New(`requirements requires the "import" operation`)
	}
	if len(args) < 2 || strings.HasPrefix(args[1], "-") {
		return parsed, errors.New("requirements import requires one work-item ID")
	}
	parsed.WorkItemID = args[1]
	fromSeen := false
	for index := 2; index < len(args); index++ {
		switch args[index] {
		case "--from":
			if fromSeen {
				return parsed, errors.New("--from may be supplied only once")
			}
			if index+1 >= len(args) {
				return parsed, errors.New("--from requires a value")
			}
			index++
			parsed.SourcePath = args[index]
			if strings.TrimSpace(parsed.SourcePath) == "" {
				return parsed, errors.New("--from must not be empty")
			}
			fromSeen = true
		case "--stdin":
			if parsed.Stdin {
				return parsed, errors.New("--stdin may be supplied only once")
			}
			parsed.Stdin = true
		case "--json":
			continue
		case "-h", "--help":
			parsed.Help = true
			return parsed, nil
		default:
			return parsed, fmt.Errorf("unknown option %q", args[index])
		}
	}
	if fromSeen == parsed.Stdin {
		return parsed, errors.New(
			"select exactly one requirement input: --from PATH or --stdin",
		)
	}
	return parsed, nil
}

func readRequirementSource(root, relative string) ([]byte, error) {
	name, err := project.ResolveContainedPath(root, relative)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open requirement source %s: %w", relative, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat requirement source %s: %w", relative, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf(
			"requirement source %s must be a regular file",
			relative,
		)
	}
	return readLimitedRequirement(file)
}

func readRequirementInput(reader io.Reader) ([]byte, error) {
	if reader == nil {
		return nil, errors.New("stdin is unavailable")
	}
	return readLimitedRequirement(reader)
}

func readLimitedRequirement(reader io.Reader) ([]byte, error) {
	content, err := io.ReadAll(
		io.LimitReader(reader, maxRequirementImportBytes+1),
	)
	if err != nil {
		return nil, fmt.Errorf("read requirement input: %w", err)
	}
	if len(content) > maxRequirementImportBytes {
		return nil, fmt.Errorf(
			"requirement input exceeds %d bytes",
			maxRequirementImportBytes,
		)
	}
	return content, nil
}

func makeInlineRequirement(id string, content []byte) []byte {
	result := make([]byte, 0, len(id)+len(content)+4)
	result = append(result, "# "...)
	result = append(result, id...)
	result = append(result, '\n', '\n')
	result = append(result, content...)
	return result
}

func importedRequirementDocument(
	id, sourceKind, sourceLabel, sourceHash string,
	requirement []byte,
) []byte {
	header := fmt.Sprintf(
		"---\nHigurashi-Requirement-Schema: 1\nWork-Item: %s\nSource-Kind: %s\nSource: %s\nSource-SHA256: %s\n---\n",
		id,
		sourceKind,
		strconv.QuoteToASCII(sourceLabel),
		sourceHash,
	)
	content := make([]byte, 0, len(header)+len(requirement))
	content = append(content, header...)
	content = append(content, requirement...)
	return content
}

var errRequirementConflict = errors.New(
	"managed requirement snapshot already exists with different content",
)

func createRequirementSnapshot(
	root, relative string,
	content []byte,
) (bool, error) {
	name, err := project.ResolveContainedPath(root, relative)
	if err != nil {
		return false, err
	}
	if err := project.ValidateMutationPath(root, name); err != nil {
		return false, err
	}
	existing, err := os.ReadFile(name)
	if err == nil {
		if bytes.Equal(existing, content) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %s", errRequirementConflict, relative)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("read managed requirement %s: %w", relative, err)
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return false, fmt.Errorf("create managed requirement directory: %w", err)
	}
	if err := project.ValidateMutationPath(root, name); err != nil {
		return false, err
	}
	temporary, err := os.CreateTemp(
		filepath.Dir(name),
		"."+filepath.Base(name)+".tmp-*",
	)
	if err != nil {
		return false, fmt.Errorf("create temporary requirement: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("write temporary requirement: %w", err)
	}
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("set requirement permissions: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return false, fmt.Errorf("sync temporary requirement: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return false, fmt.Errorf("close temporary requirement: %w", err)
	}
	if err := os.Link(temporaryName, name); err != nil {
		if errors.Is(err, os.ErrExist) {
			existing, readErr := os.ReadFile(name)
			if readErr == nil && bytes.Equal(existing, content) {
				return false, nil
			}
			return false, fmt.Errorf("%w: %s", errRequirementConflict, relative)
		}
		return false, fmt.Errorf("create managed requirement: %w", err)
	}
	return true, nil
}

func writeRequirementsImportResult(
	stdout io.Writer,
	stderr io.Writer,
	jsonOutput bool,
	envelope result.Envelope,
) int {
	if jsonOutput {
		if err := result.WriteJSON(stdout, envelope); err != nil {
			fmt.Fprintf(stderr, "higurashi: write JSON result: %v\n", err)
			return exitInternalError
		}
		return exitSuccess
	}
	fmt.Fprintf(stdout, "Requirement import: %s\n", envelope.Kind)
	fmt.Fprintf(stdout, "Work item: %s\n", envelope.WorkItemID)
	fmt.Fprintf(stdout, "Requirement source: %s\n", envelope.RequirementSource)
	fmt.Fprintf(stdout, "Source kind: %s\n", envelope.SourceKind)
	fmt.Fprintf(stdout, "Source SHA-256: %s\n", envelope.SourceHash)
	fmt.Fprintln(stdout, "Next command:")
	fmt.Fprintf(stdout, "  %s\n", envelope.NextCommands[0])
	return exitSuccess
}

func writeRequirementsHelp(writer io.Writer) {
	fmt.Fprintln(writer, `Usage:
  higurashi requirements import WORK-123 --from PATH [--json]
  higurashi requirements import WORK-123 --stdin [--json]

Imports one exact requirement into the project-managed requirement directory.
File imports extract the unique Markdown section whose heading begins with the
work-item ID. Stdin imports also accept headingless text and add only the
required ID heading. The original bytes are preserved beneath machine-owned
provenance metadata.

Import activates the managed requirement directory as the authoritative source.
An existing differing snapshot is never overwritten.`)
}
