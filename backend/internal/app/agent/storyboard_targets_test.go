package agent

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	canvas "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/application"
	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
	app "github.com/StephenQiu30/glowloom-server/backend/internal/agent/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
)

func TestFreezeStoryboardTargetsResolvesOrdinalAndExplicitRevision(t *testing.T) {
	last := canvasdomain.StoryboardShot{ID: uuid.New(), Revision: 3}
	first := canvasdomain.StoryboardShot{ID: uuid.New(), Revision: 2}
	board := &canvas.StoryboardBoard{ID: uuid.New(), Shots: []canvasdomain.StoryboardShot{first, last}}
	pins, err := freezeStoryboardTargets(board, "改第二镜的动作，加亮第 1 镜。shot 2")
	require.NoError(t, err)
	require.Equal(t, []app.StoryboardTarget{
		{Ordinal: 2, BoardID: board.ID, ShotID: last.ID, ShotRevision: 3},
		{Ordinal: 1, BoardID: board.ID, ShotID: first.ID, ShotRevision: 2},
	}, pins)

	board.Shots = []canvasdomain.StoryboardShot{last, first}
	reference := fmt.Sprintf("参考第 2 镜（board_id=%s, shot_id=%s, shot_revision=2）「片名」然后改这镜的动作。", board.ID, last.ID)
	current, err := freezeStoryboardTargets(board, reference)
	require.NoError(t, err)
	require.Equal(t, []app.StoryboardTarget{{Ordinal: 2, BoardID: board.ID, ShotID: last.ID, ShotRevision: 2}}, current)
	require.Equal(t, 3, int(pins[0].ShotRevision), "previously admitted pins retain their revision")
	_, err = freezeStoryboardTargets(board, fmt.Sprintf("参考第 1 镜（board_id=%s, shot_id=%s, shot_revision=4）", board.ID, last.ID))
	require.ErrorIs(t, err, domain.ErrConflict)
	require.Equal(t, 23, parseShotOrdinal("二十三"))
	require.Equal(t, 0, parseShotOrdinal("十十"))
}
