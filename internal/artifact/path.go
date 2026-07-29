package artifact

import (
	"fmt"
	"path"

	"github.com/jpmartinez/higurashi-loop/internal/project"
)

// ResolvePath returns the stable project-relative artifact path and its safe
// absolute equivalent. The caller must validate the work item ID first.
func ResolvePath(
	root string,
	directory string,
	workItemID string,
) (string, string, error) {
	relative := path.Join(directory, workItemID+".md")
	absolute, err := project.ResolveContainedPath(root, relative)
	if err != nil {
		return "", "", fmt.Errorf("resolve artifact path: %w", err)
	}
	return relative, absolute, nil
}
