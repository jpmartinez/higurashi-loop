package repair

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/jpmartinez/higurashi-loop/internal/artifact"
)

var (
	ErrAuthorization = errors.New("repair authorization rejected")
	ErrConflict      = errors.New("repair authorization conflict")
	ErrPartial       = errors.New("repair authorization partially applied")
)

type Authorization struct {
	Document     artifact.Document
	Handoff      Document
	Changed      bool
	Recovered    bool
	ArtifactHash string
}

type atomicWriter func(string, []byte, fs.FileMode) error

// Authorize validates the complete repair plan, consumes its handoff, and
// advances the artifact. A repeated call completes or confirms the same round.
func Authorize(
	artifactName string,
	handoffName string,
	workItemID string,
) (Authorization, error) {
	return authorizeWithWriter(
		artifactName,
		handoffName,
		workItemID,
		artifact.WriteAtomic,
	)
}

func authorizeWithWriter(
	artifactName string,
	handoffName string,
	workItemID string,
	write atomicWriter,
) (Authorization, error) {
	snapshot, err := artifact.Load(artifactName, workItemID)
	if err != nil {
		return Authorization{}, err
	}
	currentRound := snapshot.Document.RepairRound
	expectedRound := currentRound + 1
	if snapshot.Document.Status == "implementing" && currentRound > 0 {
		expectedRound = currentRound
	}
	handoff, err := Load(handoffName, workItemID, expectedRound)
	if err != nil {
		return Authorization{}, err
	}

	if snapshot.Document.Status == "implementing" &&
		handoff.Document.Status == "consumed" &&
		handoff.Document.Round == currentRound {
		return Authorization{
			Document:     snapshot.Document,
			Handoff:      handoff.Document,
			ArtifactHash: snapshot.Document.Hash,
		}, nil
	}
	if snapshot.Document.Status != "blocked" {
		return Authorization{}, fmt.Errorf(
			"%w: work item must have Status blocked",
			ErrAuthorization,
		)
	}
	if handoff.Document.Status != "ready" &&
		handoff.Document.Status != "consumed" {
		return Authorization{}, fmt.Errorf(
			"%w: handoff must be ready or recoverably consumed",
			ErrAuthorization,
		)
	}
	if err := validateRepairTasks(snapshot.Document, handoff.Document); err != nil {
		return Authorization{}, err
	}

	recovered := handoff.Document.Status == "consumed"
	if handoff.Document.Status == "ready" {
		consumed, err := consume(handoff.Content)
		if err != nil {
			return Authorization{}, err
		}
		if _, err := Parse(consumed, workItemID, expectedRound); err != nil {
			return Authorization{}, fmt.Errorf(
				"validate consumed repair handoff: %w",
				err,
			)
		}
		if err := write(handoffName, consumed, handoff.Mode); err != nil {
			return Authorization{}, fmt.Errorf("consume repair handoff: %w", err)
		}
	}

	reloaded, err := artifact.Load(artifactName, workItemID)
	if err != nil {
		return Authorization{}, fmt.Errorf(
			"%w: handoff is consumed but artifact reload failed: %v",
			ErrPartial,
			err,
		)
	}
	if reloaded.Document.Hash != snapshot.Document.Hash {
		return Authorization{}, fmt.Errorf(
			"%w: artifact changed while consuming repair handoff",
			ErrConflict,
		)
	}
	change, err := artifact.AuthorizeRepair(reloaded, expectedRound)
	if err != nil {
		return Authorization{}, fmt.Errorf("%w: %v", ErrAuthorization, err)
	}
	if err := write(artifactName, change.Content, reloaded.Mode); err != nil {
		return Authorization{}, fmt.Errorf(
			"%w: handoff is consumed; rerun %q: %v",
			ErrPartial,
			handoff.Document.NextCommand,
			err,
		)
	}

	finalArtifact, err := artifact.Load(artifactName, workItemID)
	if err != nil {
		return Authorization{}, fmt.Errorf(
			"%w: validate authorized artifact: %v",
			ErrPartial,
			err,
		)
	}
	finalHandoff, err := Load(handoffName, workItemID, expectedRound)
	if err != nil {
		return Authorization{}, fmt.Errorf(
			"%w: validate consumed handoff: %v",
			ErrPartial,
			err,
		)
	}
	if finalArtifact.Document.Status != "implementing" ||
		finalArtifact.Document.RepairRound != expectedRound ||
		finalHandoff.Document.Status != "consumed" {
		return Authorization{}, fmt.Errorf(
			"%w: authorization postcondition failed",
			ErrPartial,
		)
	}
	return Authorization{
		Document:     finalArtifact.Document,
		Handoff:      finalHandoff.Document,
		Changed:      true,
		Recovered:    recovered,
		ArtifactHash: finalArtifact.Document.Hash,
	}, nil
}

func validateRepairTasks(
	document artifact.Document,
	handoff Document,
) error {
	pending := make(map[string]artifact.Task)
	for _, task := range document.Tasks {
		if task.Complete {
			continue
		}
		pending[task.ID] = task
	}
	if len(pending) == 0 {
		return fmt.Errorf(
			"%w: no new pending repair tasks were appended",
			ErrAuthorization,
		)
	}
	if len(pending) != len(handoff.Blockers) {
		return fmt.Errorf(
			"%w: expected exactly one pending repair task per blocker",
			ErrAuthorization,
		)
	}
	for _, blocker := range handoff.Blockers {
		taskID := fmt.Sprintf("repair-r%d-%s", handoff.Round, blocker.ID)
		task, ok := pending[taskID]
		if !ok {
			return fmt.Errorf(
				"%w: missing pending task %q for blocker %q",
				ErrAuthorization,
				taskID,
				blocker.ID,
			)
		}
		required := []string{
			"Blocker-ID: " + blocker.ID,
			"Reproduction: " + blocker.Reproduction,
			"Minimum-Acceptance: " + blocker.MinimumAcceptance,
		}
		for _, evidence := range required {
			if !strings.Contains(task.Text, evidence) {
				return fmt.Errorf(
					"%w: task %q is missing exact %q",
					ErrAuthorization,
					taskID,
					evidence,
				)
			}
		}
	}
	return nil
}

func consume(content []byte) ([]byte, error) {
	ready := []byte("Handoff-Status: ready")
	consumed := []byte("Handoff-Status: consumed")
	match := -1
	for offset := 0; offset < len(content); {
		end := bytes.IndexByte(content[offset:], '\n')
		if end < 0 {
			end = len(content) - offset
		}
		line := bytes.TrimSuffix(content[offset:offset+end], []byte{'\r'})
		if bytes.Equal(line, ready) {
			if match >= 0 {
				return nil, fmt.Errorf(
					"%w: ready handoff status is not unique",
					ErrInvalidHandoff,
				)
			}
			match = offset
		}
		offset += end
		if offset < len(content) && content[offset] == '\n' {
			offset++
		}
	}
	if match < 0 {
		return nil, fmt.Errorf(
			"%w: ready handoff status is missing",
			ErrInvalidHandoff,
		)
	}
	rendered := make([]byte, 0, len(content)+len(consumed)-len(ready))
	rendered = append(rendered, content[:match]...)
	rendered = append(rendered, consumed...)
	rendered = append(rendered, content[match+len(ready):]...)
	return rendered, nil
}
