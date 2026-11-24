package canvas_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
)

func TestStoryboardOperationsKeepShotIdentityAndRevisionAcrossReorder(t *testing.T) {
	doc := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Kind: "canvas", Revision: 1, Writable: true}
	key := uuid.New()
	create := domain.StoryboardCommand{ExpectedRevision: 1, Shots: []domain.StoryboardDraft{
		{Title: "码头", Action: "渡轮靠岸", DurationMS: 3000},
		{Title: "乘客", Action: "握住栏杆", DurationMS: 4000},
	}}
	first, err := domain.PrepareStoryboardOperation(doc, key, create)
	require.NoError(t, err)
	repeat, err := domain.PrepareStoryboardOperation(doc, key, create)
	require.NoError(t, err)
	require.Equal(t, first, repeat)
	doc.Nodes = []domain.Node{*first.Node}
	doc.Revision++
	board := doc.Nodes[0]
	firstShot, second := board.Config.Storyboard.Shots[0], board.Config.Storyboard.Shots[1]
	move, err := domain.PrepareStoryboardOperation(doc, uuid.New(), domain.StoryboardCommand{
		ExpectedRevision: 2, BoardID: &board.ID, Move: &domain.StoryboardMove{ShotID: second.ID, BeforeID: &firstShot.ID},
	})
	require.NoError(t, err)
	moved, err := domain.ApplyOperations(doc, []domain.Operation{move})
	require.NoError(t, err)
	require.Equal(t, []domain.StoryboardShot{second, firstShot}, moved.Document.Nodes[0].Config.Storyboard.Shots)
	moved.Document.Revision++
	change := domain.StoryboardCommand{
		ExpectedRevision: 3, BoardID: &board.ID, Edit: &domain.StoryboardEdit{ShotID: second.ID, ExpectedRevision: second.Revision, Field: "action", Value: json.RawMessage(`"向海平面望去"`)},
	}
	edit, err := domain.PrepareStoryboardOperation(moved.Document, uuid.New(), change)
	require.NoError(t, err)
	edited, err := domain.ApplyOperations(moved.Document, []domain.Operation{edit})
	require.NoError(t, err)
	newShot := edited.Document.Nodes[0].Config.Storyboard.Shots[0]
	require.Equal(t, second.ID, newShot.ID)
	require.Equal(t, second.Revision+1, newShot.Revision)
	require.Equal(t, "向海平面望去", newShot.Action)
	require.Equal(t, firstShot, edited.Document.Nodes[0].Config.Storyboard.Shots[1])
	_, err = domain.PrepareStoryboardOperation(edited.Document, uuid.New(), change)
	require.ErrorIs(t, err, domain.ErrStoryboardConflict)
}

func TestStoryboardCopyAllocatesNewShotIdentityAndClearsSourceAdoption(t *testing.T) {
	doc := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Name: "故事板", Scope: json.RawMessage(`{}`), Kind: "canvas", Revision: 1, Writable: true, Viewport: domain.Viewport{Zoom: 1}}
	op, err := domain.PrepareStoryboardOperation(doc, uuid.New(), domain.StoryboardCommand{
		ExpectedRevision: doc.Revision, Shots: []domain.StoryboardDraft{{Title: "渡轮", Action: "驶离港口", DurationMS: 4000}},
	})
	require.NoError(t, err)
	shot := &op.Node.Config.Storyboard.Shots[0]
	shot.Revision = 2
	shot.Adopted = &domain.StoryboardTake{OperationID: uuid.New(), OutputID: uuid.New(), AssetID: uuid.New(), AssetVersion: 1, ShotRevision: 1}
	doc.Nodes = []domain.Node{*op.Node}

	project, job := uuid.New(), uuid.New()
	copied, err := domain.CopyProjectDocument(doc, project, job, nil)
	require.NoError(t, err)
	retry, err := domain.CopyProjectDocument(doc, project, job, nil)
	require.NoError(t, err)
	require.Equal(t, copied, retry)
	actual := copied.Document.Nodes[0].Config.Storyboard.Shots[0]
	require.NotEqual(t, shot.ID, actual.ID)
	require.Equal(t, shot.StoryboardDraft, actual.StoryboardDraft)
	require.EqualValues(t, 1, actual.Revision)
	require.Nil(t, actual.Adopted)
	require.Equal(t, 1, copied.ClearedOperationBindings)
	require.NotNil(t, shot.Adopted)
}

func TestStoryboardAppendKeepsExistingShotsAndRevision(t *testing.T) {
	doc := domain.Document{ID: uuid.New(), ProjectID: uuid.New(), Kind: "canvas", Revision: 1, Writable: true}
	first, err := domain.PrepareStoryboardOperation(doc, uuid.New(), domain.StoryboardCommand{
		ExpectedRevision: 1, Shots: []domain.StoryboardDraft{{Title: "码头", Action: "渡轮靠岸", DurationMS: 3000}},
	})
	require.NoError(t, err)
	doc.Nodes, doc.Revision = []domain.Node{*first.Node}, 2
	boardID := first.Node.ID
	command := domain.StoryboardCommand{ExpectedRevision: 2, BoardID: &boardID,
		Append: &domain.StoryboardDraft{Title: "乘客", Action: "握住栏杆", DurationMS: 4000}}
	key := uuid.New()
	next, err := domain.PrepareStoryboardOperation(doc, key, command)
	require.NoError(t, err)
	repeated, err := domain.PrepareStoryboardOperation(doc, key, command)
	require.NoError(t, err)
	require.Equal(t, next, repeated)
	result, err := domain.ApplyOperations(doc, []domain.Operation{next})
	require.NoError(t, err)
	shots := result.Document.Nodes[0].Config.Storyboard.Shots
	require.Len(t, shots, 2)
	require.Equal(t, first.Node.Config.Storyboard.Shots[0], shots[0])
	require.Equal(t, *command.Append, shots[1].StoryboardDraft)
	require.EqualValues(t, 1, shots[1].Revision)
	result.Document.Revision++
	_, err = domain.PrepareStoryboardOperation(result.Document, key, command)
	require.ErrorIs(t, err, domain.ErrStoryboardConflict)
	command.Edit = &domain.StoryboardEdit{}
	_, err = domain.PrepareStoryboardOperation(doc, uuid.New(), command)
	require.ErrorIs(t, err, domain.ErrInvalidCommand)
}
