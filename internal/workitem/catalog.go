package workitem

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jpmartinez/higurashi-loop/internal/project"
)

const maxRequirementBytes = 4 << 20

var (
	ErrUnknown       = errors.New("unknown work item")
	ErrMissingSource = errors.New("requirement source missing")
	ErrConflict      = errors.New("work item appears in multiple requirement sources")
)

type Match struct {
	ID     string
	Source string
}

// Extract returns the exact Markdown bytes belonging to one ATX work-item
// section. The section ends at the next heading of the same or higher level.
func Extract(data []byte, id string) ([]byte, error) {
	matchCount := 0
	start := -1
	end := len(data)
	targetLevel := 0
	inFence := false
	fenceMarker := ""

	for lineStart := 0; lineStart < len(data); {
		lineEnd := bytes.IndexByte(data[lineStart:], '\n')
		if lineEnd < 0 {
			lineEnd = len(data)
		} else {
			lineEnd += lineStart + 1
		}
		lineContentEnd := lineEnd
		if lineContentEnd > lineStart && data[lineContentEnd-1] == '\n' {
			lineContentEnd--
		}
		if lineContentEnd > lineStart && data[lineContentEnd-1] == '\r' {
			lineContentEnd--
		}
		line := string(data[lineStart:lineContentEnd])
		trimmed := strings.TrimSpace(line)
		if marker := markdownFence(trimmed); marker != "" {
			if !inFence {
				inFence = true
				fenceMarker = marker
			} else if marker == fenceMarker {
				inFence = false
				fenceMarker = ""
			}
			lineStart = lineEnd
			continue
		}
		if !inFence {
			level := headingLevel(line)
			if level > 0 {
				if headingStartsWithID(line, id) {
					matchCount++
					if matchCount == 1 {
						start = lineStart
						targetLevel = level
					}
				} else if start >= 0 &&
					end == len(data) &&
					level <= targetLevel {
					end = lineStart
				}
			}
		}
		lineStart = lineEnd
	}

	switch matchCount {
	case 0:
		return nil, fmt.Errorf("%w: %s", ErrUnknown, id)
	case 1:
		return append([]byte(nil), data[start:end]...), nil
	default:
		return nil, fmt.Errorf("%w: %s appears more than once", ErrConflict, id)
	}
}

// ValidateSources proves that every configured requirement source exists and
// can be enumerated without searching for a particular work item.
func ValidateSources(root string, sources []string) error {
	for _, source := range sources {
		if _, err := markdownFiles(root, source); err != nil {
			return err
		}
	}
	return nil
}

// ValidateID applies the configured pattern and the path-safety invariants
// required for using an ID as an artifact filename.
func ValidateID(pattern, id string) error {
	if id == "" {
		return errors.New("work item ID must not be empty")
	}
	if strings.ContainsRune(id, 0) ||
		strings.ContainsAny(id, `/\`) ||
		strings.Contains(id, "..") {
		return fmt.Errorf("unsafe work item ID %q", id)
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("compile work item ID pattern: %w", err)
	}
	if !compiled.MatchString(id) {
		return fmt.Errorf(
			"work item ID %q does not match configured pattern",
			id,
		)
	}
	return nil
}

// Find locates an exact work item ID in configured Markdown requirement
// sources. A match is an ATX heading whose first token is the ID.
func Find(root string, sources []string, id string) (Match, error) {
	var matches []string
	for _, source := range sources {
		files, err := markdownFiles(root, source)
		if err != nil {
			return Match{}, err
		}
		for _, file := range files {
			count, err := fileHeadingCount(file.absolute, id)
			if err != nil {
				return Match{}, fmt.Errorf(
					"inspect requirement source %s: %w",
					file.relative,
					err,
				)
			}
			for occurrence := 0; occurrence < count; occurrence++ {
				matches = append(matches, file.relative)
			}
		}
	}

	switch len(matches) {
	case 0:
		return Match{}, fmt.Errorf("%w: %s", ErrUnknown, id)
	case 1:
		return Match{ID: id, Source: matches[0]}, nil
	default:
		return Match{}, fmt.Errorf(
			"%w: %s found in %s",
			ErrConflict,
			id,
			strings.Join(matches, ", "),
		)
	}
}

type requirementFile struct {
	absolute string
	relative string
}

func markdownFiles(root, source string) ([]requirementFile, error) {
	absolute, err := project.ResolveContainedPath(root, source)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrMissingSource, source)
		}
		return nil, fmt.Errorf("stat requirement source %s: %w", source, err)
	}
	if !info.IsDir() {
		return []requirementFile{{absolute: absolute, relative: source}}, nil
	}

	var files []requirementFile
	err = filepath.WalkDir(absolute, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		files = append(files, requirementFile{
			absolute: name,
			relative: filepath.ToSlash(relative),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk requirement source %s: %w", source, err)
	}
	sort.Slice(files, func(left, right int) bool {
		return files[left].relative < files[right].relative
	})
	return files, nil
}

func fileHeadingCount(name, id string) (int, error) {
	file, err := os.Open(name)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxRequirementBytes+1))
	if err != nil {
		return 0, err
	}
	if len(data) > maxRequirementBytes {
		return 0, fmt.Errorf(
			"file exceeds %d bytes",
			maxRequirementBytes,
		)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), maxRequirementBytes+1)
	inFence := false
	fenceMarker := ""
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if marker := markdownFence(trimmed); marker != "" {
			if !inFence {
				inFence = true
				fenceMarker = marker
			} else if marker == fenceMarker {
				inFence = false
				fenceMarker = ""
			}
			continue
		}
		if !inFence && headingStartsWithID(line, id) {
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return count, nil
}

func markdownFence(line string) string {
	if strings.HasPrefix(line, "```") {
		return "```"
	}
	if strings.HasPrefix(line, "~~~") {
		return "~~~"
	}
	return ""
}

func headingStartsWithID(line, id string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	level := headingLevel(line)
	if level == 0 {
		return false
	}
	body := strings.TrimLeft(trimmed[level:], " \t")
	if !strings.HasPrefix(body, id) {
		return false
	}
	if len(body) == len(id) {
		return true
	}
	next, _ := utf8.DecodeRuneInString(body[len(id):])
	return unicode.IsSpace(next) ||
		next == ':' ||
		next == '—' ||
		next == '–' ||
		next == '-'
}

func headingLevel(line string) int {
	trimmed := strings.TrimLeft(line, " \t")
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level < 1 || level > 6 || level >= len(trimmed) ||
		(trimmed[level] != ' ' && trimmed[level] != '\t') {
		return 0
	}
	return level
}
