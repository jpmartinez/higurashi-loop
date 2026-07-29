package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jpmartinez/higurashi-loop/internal/project"
)

const (
	manifestSchemaVersion = 1
	manifestPath          = ".higurashi/generated.json"
)

type manifest struct {
	Generated generatedMarker `json:"generated"`
	Schema    int             `json:"schemaVersion"`
	Files     []manifestFile  `json:"files"`
}

type generatedMarker struct {
	Generator string `json:"generator"`
}

type manifestFile struct {
	Path             string `json:"path"`
	Adapter          string `json:"adapter"`
	TemplateID       string `json:"templateId"`
	SourceHash       string `json:"sourceHash"`
	ContentHash      string `json:"contentHash"`
	GeneratorVersion string `json:"generatorVersion"`
}

// ApplyOptions controls narrowly scoped generated-file replacement behavior.
type ApplyOptions struct {
	ForceGenerated bool
}

// Diff reports generated-file drift without modifying the project.
func Diff(root string, bundle Bundle) (Report, error) {
	if err := ValidateBundle(bundle); err != nil {
		return Report{}, err
	}
	current, err := loadManifest(root)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return buildReport(root, bundle, current)
}

// Apply writes a bundle only when every existing target is recognized and
// unchanged from its recorded generated content.
func Apply(root string, bundle Bundle) (Report, error) {
	return ApplyWithOptions(root, bundle, ApplyOptions{})
}

// PreflightApply proves that ApplyWithOptions can own every target without
// writing project files. A concurrent filesystem change may still make the
// subsequent apply fail closed.
func PreflightApply(
	root string,
	bundle Bundle,
	options ApplyOptions,
) (Report, error) {
	_, report, err := prepareApply(root, bundle, options)
	return report, err
}

// ApplyWithOptions writes a bundle after validating ordinary or explicitly
// forced generated-file ownership.
func ApplyWithOptions(
	root string,
	bundle Bundle,
	options ApplyOptions,
) (Report, error) {
	current, report, err := prepareApply(root, bundle, options)
	if err != nil {
		return report, err
	}

	statusByPath := make(map[string]string, len(report.Files))
	for _, status := range report.Files {
		statusByPath[status.Path] = status.Status
	}
	for _, file := range bundle.Files {
		status := statusByPath[file.Path]
		if status != "missing" && status != "update" {
			continue
		}
		name, err := safeTarget(root, file.Path)
		if err != nil {
			return report, err
		}
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return report, fmt.Errorf(
				"create generated file directory: %w",
				err,
			)
		}
		mode := os.FileMode(0o644)
		if info, statErr := os.Stat(name); statErr == nil {
			mode = info.Mode().Perm()
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return report, fmt.Errorf("stat generated file %s: %w", file.Path, statErr)
		}
		if err := writeFileAtomic(name, file.Content, mode); err != nil {
			return report, fmt.Errorf("write generated file %s: %w", file.Path, err)
		}
	}

	next := updateManifest(current, bundle)
	manifestContent, err := encodeManifest(next)
	if err != nil {
		return report, err
	}
	name, err := safeTarget(root, manifestPath)
	if err != nil {
		return report, err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return report, fmt.Errorf("create manifest directory: %w", err)
	}
	existingManifest, readErr := os.ReadFile(name)
	if readErr == nil && bytes.Equal(existingManifest, manifestContent) {
		return report, nil
	}
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return report, fmt.Errorf("read generated manifest: %w", readErr)
	}
	mode := os.FileMode(0o644)
	if info, statErr := os.Stat(name); statErr == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return report, fmt.Errorf("stat generated manifest: %w", statErr)
	}
	if err := writeFileAtomic(name, manifestContent, mode); err != nil {
		return report, fmt.Errorf("write generated manifest: %w", err)
	}
	if !contains(report.ChangedPaths, manifestPath) {
		report.ChangedPaths = append(report.ChangedPaths, manifestPath)
		sort.Strings(report.ChangedPaths)
	}
	return report, nil
}

func prepareApply(
	root string,
	bundle Bundle,
	options ApplyOptions,
) (manifest, Report, error) {
	if err := ValidateBundle(bundle); err != nil {
		return manifest{}, Report{}, err
	}
	current, err := loadManifest(root)
	if err != nil {
		return manifest{}, Report{}, fmt.Errorf("%w: %v", ErrConflict, err)
	}
	report, err := buildReport(root, bundle, current)
	if err != nil {
		return manifest{}, report, err
	}
	if len(report.Conflicts) != 0 {
		report, err = allowForcedGenerated(
			root,
			bundle,
			current,
			report,
			options,
		)
		if err != nil {
			return manifest{}, report, err
		}
	}
	return current, report, nil
}

func allowForcedGenerated(
	root string,
	bundle Bundle,
	current manifest,
	report Report,
	options ApplyOptions,
) (Report, error) {
	if !options.ForceGenerated {
		return report, fmt.Errorf("%w: %v", ErrConflict, report.Conflicts)
	}
	records := make(map[string]manifestFile, len(current.Files))
	for _, record := range current.Files {
		records[record.Path] = record
	}
	files := make(map[string]File, len(bundle.Files))
	for _, file := range bundle.Files {
		files[file.Path] = file
	}
	forced := make(map[string]bool, len(report.Conflicts))
	for _, relative := range report.Conflicts {
		record, recorded := records[relative]
		file, targeted := files[relative]
		if !recorded || !targeted || record.TemplateID != file.TemplateID {
			return report, fmt.Errorf("%w: %v", ErrConflict, report.Conflicts)
		}
		name, err := safeTarget(root, relative)
		if err != nil {
			return report, err
		}
		content, err := os.ReadFile(name)
		if err != nil || !recognizedGeneratedContent(content, record) {
			return report, fmt.Errorf("%w: %v", ErrConflict, report.Conflicts)
		}
		forced[relative] = true
	}
	for index := range report.Files {
		if forced[report.Files[index].Path] {
			report.Files[index].Status = "update"
		}
	}
	for relative := range forced {
		if !contains(report.ChangedPaths, relative) {
			report.ChangedPaths = append(report.ChangedPaths, relative)
		}
	}
	sort.Strings(report.ChangedPaths)
	report.Conflicts = nil
	return report, nil
}

func buildReport(
	root string,
	bundle Bundle,
	current manifest,
) (Report, error) {
	report := Report{Adapter: bundle.Adapter}
	records := make(map[string]manifestFile, len(current.Files))
	for _, record := range current.Files {
		records[record.Path] = record
	}
	currentPaths := make(map[string]bool, len(bundle.Files))

	for _, file := range bundle.Files {
		currentPaths[file.Path] = true
		name, err := safeTarget(root, file.Path)
		if err != nil {
			return report, err
		}
		actual, readErr := os.ReadFile(name)
		status := "unchanged"
		switch {
		case errors.Is(readErr, os.ErrNotExist):
			status = "missing"
			report.ChangedPaths = append(report.ChangedPaths, file.Path)
		case readErr != nil:
			status = "conflict"
			report.Conflicts = append(report.Conflicts, file.Path)
		case bytes.Equal(actual, file.Content):
			if _, exists := records[file.Path]; !exists {
				status = "adopt"
			}
		default:
			record, exists := records[file.Path]
			if !exists ||
				hashBytes(actual) != record.ContentHash ||
				!recognizedGeneratedContent(actual, record) {
				status = "local_modification"
				report.Conflicts = append(report.Conflicts, file.Path)
			} else {
				status = "update"
				report.ChangedPaths = append(report.ChangedPaths, file.Path)
			}
		}
		report.Files = append(report.Files, FileStatus{
			Path:   file.Path,
			Status: status,
		})
	}

	for _, record := range current.Files {
		if record.Adapter == bundle.Adapter && !currentPaths[record.Path] {
			report.Stale = append(report.Stale, record.Path)
			report.Files = append(report.Files, FileStatus{
				Path:   record.Path,
				Status: "stale",
			})
		}
	}
	sort.Slice(report.Files, func(left, right int) bool {
		return report.Files[left].Path < report.Files[right].Path
	})
	sort.Strings(report.ChangedPaths)
	sort.Strings(report.Conflicts)
	sort.Strings(report.Stale)
	return report, nil
}

func loadManifest(root string) (manifest, error) {
	name, err := safeTarget(root, manifestPath)
	if err != nil {
		return manifest{}, err
	}
	file, err := os.Open(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newManifest(), nil
		}
		return manifest{}, fmt.Errorf("open generated manifest: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return manifest{}, fmt.Errorf("read generated manifest: %w", err)
	}
	if len(data) > 1<<20 {
		return manifest{}, errors.New("generated manifest exceeds 1048576 bytes")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value manifest
	if err := decoder.Decode(&value); err != nil {
		return manifest{}, fmt.Errorf("decode generated manifest: %w", err)
	}
	if value.Schema != manifestSchemaVersion ||
		value.Generated.Generator != "higurashi" {
		return manifest{}, errors.New("unrecognized generated manifest")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return manifest{}, errors.New("generated manifest has trailing content")
	}
	if err := validateManifest(value); err != nil {
		return manifest{}, err
	}
	return value, nil
}

func newManifest() manifest {
	return manifest{
		Generated: generatedMarker{Generator: "higurashi"},
		Schema:    manifestSchemaVersion,
		Files:     []manifestFile{},
	}
}

func updateManifest(current manifest, bundle Bundle) manifest {
	records := make(map[string]manifestFile, len(current.Files)+len(bundle.Files))
	for _, record := range current.Files {
		records[record.Path] = record
	}
	for _, file := range bundle.Files {
		records[file.Path] = manifestFile{
			Path:             file.Path,
			Adapter:          bundle.Adapter,
			TemplateID:       file.TemplateID,
			SourceHash:       file.SourceHash,
			ContentHash:      hashBytes(file.Content),
			GeneratorVersion: bundle.GeneratorVersion,
		}
	}
	next := newManifest()
	for _, record := range records {
		next.Files = append(next.Files, record)
	}
	sort.Slice(next.Files, func(left, right int) bool {
		return next.Files[left].Path < next.Files[right].Path
	})
	return next
}

func encodeManifest(value manifest) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("encode generated manifest: %w", err)
	}
	return output.Bytes(), nil
}

func validateManifest(value manifest) error {
	previousPath := ""
	for index, record := range value.Files {
		if record.Path == "" ||
			path.IsAbs(record.Path) ||
			path.Clean(record.Path) != record.Path ||
			record.Path == ".." ||
			strings.HasPrefix(record.Path, "../") {
			return fmt.Errorf("generated manifest files[%d].path is unsafe", index)
		}
		if previousPath != "" && record.Path <= previousPath {
			return errors.New(
				"generated manifest file paths must be unique and sorted",
			)
		}
		previousPath = record.Path
		if record.Adapter != "opencode" && record.Adapter != "claude-code" {
			return fmt.Errorf(
				"generated manifest files[%d].adapter is invalid",
				index,
			)
		}
		if record.TemplateID == "" || record.GeneratorVersion == "" ||
			!validHash(record.SourceHash) ||
			!validHash(record.ContentHash) {
			return fmt.Errorf(
				"generated manifest files[%d] has invalid ownership metadata",
				index,
			)
		}
	}
	return nil
}

func recognizedGeneratedContent(content []byte, record manifestFile) bool {
	values := []string{
		"generator=higurashi",
		"version=" + record.GeneratorVersion,
		"template=" + record.TemplateID,
		"source-sha256=" + record.SourceHash,
	}
	text := string(content)
	for _, value := range values {
		if !strings.Contains(text, value) {
			return false
		}
	}
	return true
}

func validHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') &&
			!(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func safeTarget(root, relative string) (string, error) {
	if relative == ".git" ||
		len(relative) > len(".git/") && relative[:len(".git/")] == ".git/" {
		return "", fmt.Errorf("generated path must not target Git metadata: %s", relative)
	}
	name, err := project.ResolveContainedPath(root, relative)
	if err != nil {
		return "", err
	}
	if err := project.ValidateMutationPath(root, name); err != nil {
		return "", err
	}
	return name, nil
}

func writeFileAtomic(name string, content []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(
		filepath.Dir(name),
		"."+filepath.Base(name)+".tmp-*",
	)
	if err != nil {
		return err
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
	if err := os.Rename(temporaryName, name); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
