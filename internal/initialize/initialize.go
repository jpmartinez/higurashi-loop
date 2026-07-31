package initialize

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"

	"github.com/jpmartinez/higurashi-loop/internal/config"
	"github.com/jpmartinez/higurashi-loop/internal/project"
	"github.com/jpmartinez/higurashi-loop/internal/render"
	"github.com/jpmartinez/higurashi-loop/internal/workitem"
)

var (
	ErrConfigurationConflict = errors.New("initialization configuration conflict")
	ErrInvalidConfiguration  = errors.New("invalid initialization configuration")
)

type Request struct {
	Runners            []string
	RequirementSources []string
	GeneratorVersion   string
	ForceGenerated     bool
}

type Result struct {
	Config       config.Config
	ChangedPaths []string
	Conflicts    []string
}

// Apply initializes one already-resolved Git project. Every generated target
// is preflighted before project state is created.
func Apply(root string, request Request) (Result, error) {
	configuration, configMissing, err := loadOrPrepareConfig(
		root,
		request.Runners,
		request.RequirementSources,
	)
	if err != nil {
		return Result{}, err
	}
	result := Result{Config: configuration}
	if len(request.RequirementSources) != 0 {
		if err := workitem.ValidateSources(
			root,
			configuration.WorkItems.RequirementSources,
		); err != nil {
			return result, fmt.Errorf(
				"%w: requirement sources: %v",
				ErrInvalidConfiguration,
				err,
			)
		}
	}

	options := render.ApplyOptions{ForceGenerated: request.ForceGenerated}
	bundles := make([]render.Bundle, 0, len(request.Runners))
	for _, runner := range request.Runners {
		runnerConfiguration, ok := configuration.Runner(runner)
		if !ok {
			return result, fmt.Errorf("unsupported runner %q", runner)
		}
		bundle, buildErr := render.BuildConfigured(
			runner,
			request.GeneratorVersion,
			runnerConfiguration.Models,
		)
		if buildErr != nil {
			return result, buildErr
		}
		report, preflightErr := render.PreflightApply(root, bundle, options)
		if preflightErr != nil {
			result.Conflicts = append(result.Conflicts, report.Conflicts...)
			sort.Strings(result.Conflicts)
			return result, preflightErr
		}
		bundles = append(bundles, bundle)
	}

	artifactDirectory, err := project.ResolveContainedPath(
		root,
		configuration.Artifacts.Directory,
	)
	if err != nil {
		return result, err
	}
	if err := project.ValidateMutationPath(root, artifactDirectory); err != nil {
		return result, err
	}
	artifactMissing, err := directoryMissing(artifactDirectory)
	if err != nil {
		return result, err
	}

	managedRequirementDirectory := path.Join(
		configuration.Artifacts.Directory,
		"requirements",
	)
	managedRequirementConfigured := slices.Contains(
		configuration.WorkItems.RequirementSources,
		managedRequirementDirectory,
	)
	if managedRequirementConfigured {
		managedRequirementPath, pathErr := project.ResolveContainedPath(
			root,
			managedRequirementDirectory,
		)
		if pathErr != nil {
			return result, pathErr
		}
		if pathErr := project.ValidateMutationPath(
			root,
			managedRequirementPath,
		); pathErr != nil {
			return result, pathErr
		}
		managedRequirementMissing, pathErr := directoryMissing(
			managedRequirementPath,
		)
		if pathErr != nil {
			return result, pathErr
		}
		if managedRequirementMissing {
			if mkdirErr := os.MkdirAll(
				managedRequirementPath,
				0o755,
			); mkdirErr != nil {
				return result, fmt.Errorf(
					"create managed requirement directory: %w",
					mkdirErr,
				)
			}
			result.ChangedPaths = append(
				result.ChangedPaths,
				managedRequirementDirectory,
			)
		}
	}
	if artifactMissing {
		if err := os.MkdirAll(artifactDirectory, 0o755); err != nil {
			return result, fmt.Errorf("create artifact directory: %w", err)
		}
		result.ChangedPaths = append(
			result.ChangedPaths,
			configuration.Artifacts.Directory,
		)
	}
	if configMissing {
		content, encodeErr := config.Encode(configuration)
		if encodeErr != nil {
			return result, encodeErr
		}
		configName, pathErr := project.ResolveContainedPath(
			root,
			".higurashi/config.json",
		)
		if pathErr != nil {
			return result, pathErr
		}
		if pathErr := project.ValidateMutationPath(root, configName); pathErr != nil {
			return result, pathErr
		}
		if mkdirErr := os.MkdirAll(filepath.Dir(configName), 0o755); mkdirErr != nil {
			return result, fmt.Errorf("create configuration directory: %w", mkdirErr)
		}
		if writeErr := createFileAtomic(configName, content, 0o644); writeErr != nil {
			return result, fmt.Errorf("create configuration: %w", writeErr)
		}
		result.ChangedPaths = append(
			result.ChangedPaths,
			".higurashi/config.json",
		)
	}

	for _, bundle := range bundles {
		report, applyErr := render.ApplyWithOptions(root, bundle, options)
		if applyErr != nil {
			result.Conflicts = append(result.Conflicts, report.Conflicts...)
			sort.Strings(result.Conflicts)
			return result, applyErr
		}
		result.ChangedPaths = append(
			result.ChangedPaths,
			report.ChangedPaths...,
		)
	}
	result.ChangedPaths = uniqueSorted(result.ChangedPaths)
	return result, nil
}

func loadOrPrepareConfig(
	root string,
	runners []string,
	requirementSources []string,
) (config.Config, bool, error) {
	name, err := project.ResolveContainedPath(root, ".higurashi/config.json")
	if err != nil {
		return config.Config{}, false, err
	}
	_, err = os.Stat(name)
	if err == nil {
		configuration, loadErr := config.Load(root)
		if loadErr != nil {
			return config.Config{}, false, fmt.Errorf(
				"%w: %v",
				ErrInvalidConfiguration,
				loadErr,
			)
		}
		for _, runner := range runners {
			enabled := (runner == "opencode" &&
				configuration.Runners.OpenCode.Enabled) ||
				(runner == "claude-code" &&
					configuration.Runners.ClaudeCode.Enabled) ||
				(runner == "pi" && configuration.Runners.Pi.Enabled)
			if !enabled {
				return config.Config{}, false, fmt.Errorf(
					"%w: selected runner %q is disabled in existing configuration",
					ErrConfigurationConflict,
					runner,
				)
			}
		}
		if len(requirementSources) != 0 &&
			!slices.Equal(
				configuration.WorkItems.RequirementSources,
				requirementSources,
			) {
			return config.Config{}, false, fmt.Errorf(
				"%w: explicit requirement sources differ from existing configuration",
				ErrConfigurationConflict,
			)
		}
		return configuration, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return config.Config{}, false, fmt.Errorf("stat configuration: %w", err)
	}

	configuration := config.Defaults()
	if len(requirementSources) != 0 {
		configuration.WorkItems.RequirementSources = append(
			[]string(nil),
			requirementSources...,
		)
	}
	configuration.Runners.OpenCode.Enabled = false
	configuration.Runners.ClaudeCode.Enabled = false
	configuration.Runners.Pi.Enabled = false
	for _, runner := range runners {
		switch runner {
		case "opencode":
			configuration.Runners.OpenCode.Enabled = true
		case "claude-code":
			configuration.Runners.ClaudeCode.Enabled = true
		case "pi":
			configuration.Runners.Pi.Enabled = true
		}
	}
	return configuration, true, nil
}

func directoryMissing(name string) (bool, error) {
	info, err := os.Stat(name)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat artifact directory: %w", err)
	}
	if !info.IsDir() {
		return false, errors.New("artifact directory path exists and is not a directory")
	}
	return false, nil
}

func createFileAtomic(name string, content []byte, mode fs.FileMode) error {
	temporary, err := os.CreateTemp(
		filepath.Dir(name),
		"."+filepath.Base(name)+".tmp-*",
	)
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode.Perm()); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Link(temporaryName, name); err != nil {
		return err
	}
	return nil
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
