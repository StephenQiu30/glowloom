package application

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain/graph"
	"github.com/google/uuid"
)

// CanvasSnapshot represents current collaborative canvas elements for diff calculation.
type CanvasSnapshot struct {
	ProjectID uuid.UUID              `json:"project_id"`
	Revision  int64                  `json:"revision"`
	Elements  map[string]ElementInfo `json:"elements"`
}

// ElementInfo describes one element on the canvas (shot, character, scene, or script segment).
type ElementInfo struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // "character", "scene", "shot", "script"
	Title     string `json:"title"`
	Content   string `json:"content"`
	UpdatedAt int64  `json:"updated_at"`
}

// StudioDigestComputer computes canvas changes against the last known studio digest (BeefTV StudioDigest pattern).
type StudioDigestComputer struct{}

// NewStudioDigestComputer instantiates a studio digest diff computer.
func NewStudioDigestComputer() *StudioDigestComputer {
	return &StudioDigestComputer{}
}

// ComputeDigestHash computes a deterministic digest of the snapshot elements.
func (c *StudioDigestComputer) ComputeDigestHash(snap CanvasSnapshot) (string, error) {
	keys := make([]string, 0, len(snap.Elements))
	for k := range snap.Elements {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("rev:%d|", snap.Revision))
	for _, k := range keys {
		el := snap.Elements[k]
		sb.WriteString(fmt.Sprintf("%s:%s:%s:%s:%d|", el.ID, el.Type, el.Title, el.Content, el.UpdatedAt))
	}
	digest := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(digest[:]), nil
}

// InjectDigestDiff compares current snapshot with state.LastStudioHash.
// If changed, formats `<canvas_changes>...</canvas_changes>` diff text and attaches it to state.
func (c *StudioDigestComputer) InjectDigestDiff(state *graph.AgentState, snap CanvasSnapshot, prevSnapshot *CanvasSnapshot) error {
	newHash, err := c.ComputeDigestHash(snap)
	if err != nil {
		return err
	}

	if state.LastStudioHash == newHash {
		// No changes since last turn
		state.StudioDiffText = ""
		return nil
	}

	diffText := c.formatDiff(snap, prevSnapshot)
	state.LastStudioHash = newHash
	state.StudioDiffText = diffText

	// If there are changes, inject `<canvas_changes>` as a system message for agent self-healing
	if diffText != "" {
		changesMsg := fmt.Sprintf("<canvas_changes>\n%s\n</canvas_changes>", diffText)
		state.Messages = append(state.Messages, graph.Message{
			ID:        uuid.New(),
			Role:      "system",
			Content:   changesMsg,
			Timestamp: time.Now().UTC(),
		})
	}
	return nil
}

func (c *StudioDigestComputer) formatDiff(current CanvasSnapshot, previous *CanvasSnapshot) string {
	if previous == nil || len(previous.Elements) == 0 {
		return fmt.Sprintf("项目画布同步: 共 %d 个元素 (rev:%d)", len(current.Elements), current.Revision)
	}

	var changes []string

	// Detect added or modified elements
	for id, currEl := range current.Elements {
		prevEl, exists := previous.Elements[id]
		if !exists {
			changes = append(changes, fmt.Sprintf("+ 新增元素 [%s] %s (id: %s)", currEl.Type, currEl.Title, currEl.ID))
		} else if currEl.Content != prevEl.Content || currEl.Title != prevEl.Title {
			changes = append(changes, fmt.Sprintf("~ 更新元素 [%s] %s (id: %s): %s", currEl.Type, currEl.Title, currEl.ID, currEl.Content))
		}
	}

	// Detect deleted elements
	for id, prevEl := range previous.Elements {
		if _, exists := current.Elements[id]; !exists {
			changes = append(changes, fmt.Sprintf("- 删除元素 [%s] %s (id: %s)", prevEl.Type, prevEl.Title, prevEl.ID))
		}
	}

	sort.Strings(changes)
	return strings.Join(changes, "\n")
}
