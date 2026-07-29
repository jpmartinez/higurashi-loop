package repair

import (
	"fmt"
	"path"
	"strconv"

	"github.com/jpmartinez/higurashi-loop/internal/project"
)

// ResolvePath returns the stable project-relative handoff path and its safe
// absolute equivalent. The caller must validate the work item ID first.
func ResolvePath(
	root string,
	artifactDirectory string,
	workItemID string,
	round int,
) (string, string, error) {
	if round <= 0 {
		return "", "", fmt.Errorf("repair round must be positive")
	}
	relative := path.Join(
		artifactDirectory,
		workItemID+"-repair-"+strconv.Itoa(round)+".md",
	)
	absolute, err := project.ResolveContainedPath(root, relative)
	if err != nil {
		return "", "", fmt.Errorf("resolve repair handoff path: %w", err)
	}
	return relative, absolute, nil
}
