package agent

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"unicode/utf8"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/creation/canvas/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

const maxStoryboardTargets = 6

var (
	storyboardOrdinal = regexp.MustCompile(`第\s*([1-9][0-9]?|[一二两三四五六七八九十]{1,3})\s*镜|(?i:\bshot\s+([1-9][0-9]?)\b)`)
	explicitShotRef   = regexp.MustCompile(`第\s*([1-9][0-9]?)\s*镜(?:「[^」\n]*」)?[（(]board_id=([a-f0-9-]{36}), shot_id=([a-f0-9-]{36}), shot_revision=([1-9][0-9]{0,18})[）)]`)
)

// StoryboardTargets freezes recognized ordinals at submission. Explicit
// references resolve their stable ID even if their displayed ordinal is old.
func (b *Boundary) StoryboardTargets(ctx context.Context, actor identity.Principal, project, id uuid.UUID, text string) ([]app.StoryboardTarget, error) {
	if !storyboardOrdinal.MatchString(text) {
		return nil, nil
	}
	view, err := canvas.NewYjsService(canvasStore(b.db), nil).ReadStoryboard(ctx, actor, id)
	if errors.Is(err, canvas.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, businessError(err)
	}
	if view.ProjectID != project {
		return nil, domain.ErrNotFound
	}
	return freezeStoryboardTargets(view.Board, text)
}

func freezeStoryboardTargets(board *canvas.StoryboardBoard, text string) ([]app.StoryboardTarget, error) {
	if board == nil {
		return nil, nil
	}
	targets := make([]app.StoryboardTarget, 0, maxStoryboardTargets)
	explicitOrdinals := map[int]bool{}
	seen := map[uuid.UUID]bool{}
	for _, ref := range explicitShotRef.FindAllStringSubmatch(text, maxStoryboardTargets+1) {
		ordinal, _ := strconv.Atoi(ref[1])
		boardID, boardErr := uuid.Parse(ref[2])
		shotID, shotErr := uuid.Parse(ref[3])
		revision, revisionErr := strconv.ParseInt(ref[4], 10, 64)
		shot, found := (canvas.StoryboardView{Board: board}).FindShot(boardID, shotID)
		if ordinal > 64 || boardErr != nil || shotErr != nil || revisionErr != nil || !found || revision > shot.Revision || revision > 9007199254740991 {
			return nil, domain.ErrConflict
		}
		explicitOrdinals[ordinal] = true
		if seen[shotID] {
			continue
		}
		seen[shotID] = true
		targets = append(targets, app.StoryboardTarget{Ordinal: ordinal, BoardID: boardID, ShotID: shotID, ShotRevision: revision})
	}
	for _, match := range storyboardOrdinal.FindAllStringSubmatch(text, -1) {
		ordinal := parseShotOrdinal(match[1] + match[2])
		if ordinal < 1 || ordinal > len(board.Shots) || explicitOrdinals[ordinal] {
			continue
		}
		shot := board.Shots[ordinal-1]
		if seen[shot.ID] {
			continue
		}
		seen[shot.ID] = true
		targets = append(targets, app.StoryboardTarget{Ordinal: ordinal, BoardID: board.ID, ShotID: shot.ID, ShotRevision: shot.Revision})
		if len(targets) > maxStoryboardTargets {
			break
		}
	}
	if len(targets) > maxStoryboardTargets {
		return nil, domain.ErrInvalid
	}
	return targets, nil
}

func parseShotOrdinal(value string) int {
	if n, err := strconv.Atoi(value); err == nil {
		return n
	}
	digit := func(r rune) int {
		if r == '两' {
			return 2
		}
		for i, d := range []rune("零一二三四五六七八九") {
			if r == d {
				return i
			}
		}
		return 0
	}
	first, size := utf8.DecodeRuneInString(value)
	rest := value[size:]
	if rest == "" && first != '十' {
		return digit(first)
	}
	tens := 1
	if first != '十' {
		tens = digit(first)
		if len(rest) < len("十") || rest[:len("十")] != "十" {
			return 0
		}
		rest = rest[len("十"):]
	}
	if rest == "" {
		return tens * 10
	}
	last, size := utf8.DecodeRuneInString(rest)
	if len(rest) != size {
		return 0
	}
	ones := digit(last)
	if ones == 0 {
		return 0
	}
	return tens*10 + ones
}
