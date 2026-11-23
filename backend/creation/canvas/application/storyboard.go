package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/canvas/domain"
	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
)

// StoryboardBoard is the ordered story content and its owning node identity.
type StoryboardBoard struct {
	ID    uuid.UUID               `json:"id"`
	Shots []domain.StoryboardShot `json:"shots"`
}

// StoryboardView reads the formal board at one canvas revision.
type StoryboardView struct {
	CanvasID  uuid.UUID        `json:"canvas_id"`
	ProjectID uuid.UUID        `json:"project_id"`
	Revision  int64            `json:"revision"`
	CommitSeq Revision         `json:"commit_seq,omitempty" swaggertype:"string"`
	Board     *StoryboardBoard `json:"board,omitempty"`
}

// FindShot resolves a stable ID in the board owned by this view.
func (v StoryboardView) FindShot(boardID, shotID uuid.UUID) (domain.StoryboardShot, bool) {
	if v.Board != nil && v.Board.ID == boardID {
		for _, shot := range v.Board.Shots {
			if shot.ID == shotID {
				return shot, true
			}
		}
	}
	return domain.StoryboardShot{}, false
}

// PreparedStoryboard holds either the original result or a candidate prepared
// outside the transaction that rechecks authorization and persists its receipt.
type PreparedStoryboard struct {
	Key     uuid.UUID
	Hash    string
	View    StoryboardView
	Command *PreparedYjsCommands
	Take    *domain.StoryboardTake
}

// StoryboardStore persists command identity with the authorized Yjs commit.
type StoryboardStore interface {
	ReadStoryboardReceipt(context.Context, identity.Principal, uuid.UUID, uuid.UUID, string) (*StoryboardView, error)
	CommitStoryboard(context.Context, identity.Principal, PreparedStoryboard) (StoryboardView, error)
}

// ReadStoryboard returns the ordinary canvas board, if there has been a formal write.
func (s *YjsService) ReadStoryboard(ctx context.Context, actor identity.Principal, id uuid.UUID) (StoryboardView, error) {
	base, err := s.Load(ctx, actor, id)
	if err != nil {
		return StoryboardView{}, err
	}
	if base.Document.Kind == "short_drama" {
		return StoryboardView{}, ErrNotFound
	}
	return storyboardView(base.CommandDocument, base.CommitSeq), nil
}

func storyboardView(doc domain.Document, seq Revision) StoryboardView {
	view := StoryboardView{CanvasID: doc.ID, ProjectID: doc.ProjectID, Revision: doc.Revision, CommitSeq: seq}
	if node, found := domain.StoryboardNode(doc); found {
		view.Board = &StoryboardBoard{ID: node.ID, Shots: node.Config.Storyboard.Shots}
	}
	return view
}

// PrepareStoryboard accepts the same semantic operation for browser and Agent.
func (s *YjsService) PrepareStoryboard(ctx context.Context, actor identity.Principal, id, key uuid.UUID, input domain.StoryboardCommand) (PreparedStoryboard, error) {
	return s.prepareStoryboard(ctx, actor, id, key, input, func(doc domain.Document, commandID uuid.UUID) (domain.Operation, error) {
		return domain.PrepareStoryboardOperation(doc, commandID, input)
	})
}

// PrepareStoryboardAdoption resolves the selected operation only after checking
// for an existing receipt and commits the same media identity with the canvas.
func (s *YjsService) PrepareStoryboardAdoption(ctx context.Context, actor identity.Principal, id, key uuid.UUID, input domain.StoryboardAdoption, resolve func(context.Context, uuid.UUID) (domain.StoryboardTake, error)) (PreparedStoryboard, error) {
	var take domain.StoryboardTake
	prepared, err := s.prepareStoryboard(ctx, actor, id, key, input, func(doc domain.Document, _ uuid.UUID) (domain.Operation, error) {
		var err error
		take, err = resolve(ctx, doc.ProjectID)
		if err != nil {
			return domain.Operation{}, err
		}
		return domain.PrepareStoryboardAdoption(doc, input, take)
	})
	if prepared.Command != nil {
		prepared.Take = &take
	}
	return prepared, err
}

func (s *YjsService) prepareStoryboard(ctx context.Context, actor identity.Principal, id, key uuid.UUID, input any, operation func(domain.Document, uuid.UUID) (domain.Operation, error)) (PreparedStoryboard, error) {
	store, ok := s.store.(StoryboardStore)
	if !ok || id == uuid.Nil || key == uuid.Nil {
		return PreparedStoryboard{}, domain.ErrInvalidCommand
	}
	body, err := json.Marshal(input)
	if err != nil {
		return PreparedStoryboard{}, err
	}
	hash := sha256.Sum256(body)
	prepared := PreparedStoryboard{Key: uuid.NewSHA1(id, []byte("storyboard:"+key.String())), Hash: hex.EncodeToString(hash[:])}
	prior, err := store.ReadStoryboardReceipt(ctx, actor, id, prepared.Key, prepared.Hash)
	if err != nil {
		return PreparedStoryboard{}, err
	}
	if prior != nil {
		prepared.View = *prior
		return prepared, nil
	}
	base, err := s.Load(ctx, actor, id)
	if err != nil {
		return PreparedStoryboard{}, err
	}
	commandID := uuid.NewSHA1(prepared.Key, []byte(actor.ID.String()))
	op, err := operation(base.CommandDocument, commandID)
	if err != nil {
		return PreparedStoryboard{}, err
	}
	commands, err := s.prepareCommands(ctx, actor, id, YjsCommandInput{
		CommandID: commandID, SessionID: uuid.NewSHA1(actor.ID, []byte("storyboard:"+id.String())),
		ExpectedRevision: base.CommandDocument.Revision, ExpectedCommitSeq: &base.CommitSeq, Operations: []domain.Operation{op},
	}, true)
	if err != nil {
		return PreparedStoryboard{}, err
	}
	projection := commands.Candidate.Projection
	prepared.View = StoryboardView{CanvasID: id, ProjectID: base.Document.ProjectID}
	for _, node := range projection.Nodes {
		if node.NodeType == domain.NodeTypeStoryboard && node.Config.Storyboard != nil {
			prepared.View.Board = &StoryboardBoard{ID: node.ID, Shots: node.Config.Storyboard.Shots}
		}
	}
	if prepared.View.Board == nil {
		return PreparedStoryboard{}, domain.ErrInvalidCommand
	}
	prepared.Command = &commands
	return prepared, nil
}

// CommitStoryboard accepts the current candidate and writes its original receipt.
func (s *YjsService) CommitStoryboard(ctx context.Context, actor identity.Principal, prepared PreparedStoryboard) (StoryboardView, error) {
	store, ok := s.store.(StoryboardStore)
	if !ok {
		return StoryboardView{}, domain.ErrInvalidCommand
	}
	if prepared.Command == nil {
		return prepared.View, nil
	}
	return store.CommitStoryboard(ctx, actor, prepared)
}
