package script_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	app "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

type sourceObjects struct {
	data      map[string][]byte
	uncertain bool
	puts      int
}

func (o *sourceObjects) PutIfAbsent(_ context.Context, key string, r io.Reader, _ int64, _, _ string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if _, ok := o.data[key]; ok {
		return app.ErrObjectExists
	}
	o.data[key] = data
	o.puts++
	if o.uncertain {
		return errors.New("lost object response")
	}
	return nil
}
func (o *sourceObjects) Get(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := o.data[key]
	if !ok {
		return nil, app.ErrObjectMissing
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}
func (o *sourceObjects) Remove(_ context.Context, key string) error { delete(o.data, key); return nil }

type sourcePersistence struct {
	base             app.SourceBase
	pending          *app.PendingWrite
	begins, finishes int
}

func (s *sourcePersistence) LoadBase(context.Context, identityapp.Principal, uuid.UUID, *uuid.UUID) (app.SourceBase, error) {
	return s.base, nil
}
func (s *sourcePersistence) FindCommand(_ context.Context, _ identityapp.Principal, _, _ uuid.UUID, hash string) (*app.PendingWrite, error) {
	if s.pending != nil && s.pending.Plan.RequestHash != hash {
		return nil, app.ErrIdempotencyConflict
	}
	return s.pending, nil
}
func (s *sourcePersistence) BeginWrite(_ context.Context, _ identityapp.Principal, plan app.WritePlan) (app.PendingWrite, error) {
	s.begins++
	if s.pending == nil {
		s.pending = &app.PendingWrite{Plan: plan}
	}
	return *s.pending, nil
}
func (s *sourcePersistence) FinishWrite(_ context.Context, _ identityapp.Principal, plan app.WritePlan) (app.SourceReceipt, error) {
	s.finishes++
	r := app.SourceReceipt{ScriptRevision: 1, ProjectRevision: 2, VersionID: plan.Version.ID, SplitSetID: plan.Candidate.ID, Mappings: plan.Mappings, Changed: true}
	s.pending.Receipt = &r
	return r, nil
}

func sourceCommand() (identityapp.Principal, app.SourceCommand) {
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New()}
	input := app.SourceCommand{ProjectID: uuid.New(), Key: uuid.New(), RequestID: uuid.New(), Action: "create", RightsConfirmed: true, Sources: []app.SourceInput{{Kind: "chapter", Title: "第一章", Status: "draft", Document: domain.RichDocument{Type: "doc", Content: []domain.RichDocument{{Type: "paragraph", Content: []domain.RichDocument{{Type: "text", Text: "😀é"}}}}}}}}
	return actor, input
}

func TestScriptWriteUnknownPutReplaysOriginalManifestWithoutDuplicateVersion(t *testing.T) {
	actor, input := sourceCommand()
	store := &sourcePersistence{}
	objects := &sourceObjects{data: make(map[string][]byte), uncertain: true}
	clock := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	service := app.NewSourceService(store, objects, func() time.Time { return clock })
	if _, err := service.Write(t.Context(), actor, input); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal("unknown write accepted", err)
	}
	if store.finishes != 0 || store.pending == nil || len(store.pending.Plan.Objects) != 4 {
		t.Fatal("unknown write published or lost manifest")
	}
	if len(store.pending.Plan.Command.Sources) != 0 {
		t.Fatal("private body persisted in command")
	}
	original := store.pending.Plan.Version
	clock = clock.Add(time.Hour)
	objects.uncertain = false
	result, err := service.Write(t.Context(), actor, input)
	if err != nil || result.VersionID != original.ID || objects.puts != 4 || store.finishes != 1 {
		t.Fatal("original unknown replay", result, objects.puts, store.finishes, err)
	}
	input.RequestID = uuid.New()
	replay, err := service.Write(t.Context(), actor, input)
	if err != nil || replay.VersionID != result.VersionID || store.finishes != 1 {
		t.Fatal("trace changed permanent receipt", replay, err)
	}
	input.Sources[0].Title = "另一个标题"
	if _, err := service.Write(t.Context(), actor, input); !errors.Is(err, app.ErrIdempotencyConflict) {
		t.Fatal("same key changed input", err)
	}
}

func TestScriptAtomicImportRejectsWholeBatchBeforeObjectOrCommand(t *testing.T) {
	actor, input := sourceCommand()
	input.Action = "import"
	input.Sources = append(input.Sources, app.SourceInput{Kind: "chapter", Title: "坏章节", Status: "draft", Document: domain.RichDocument{Type: "image"}})
	store := &sourcePersistence{}
	objects := &sourceObjects{data: make(map[string][]byte)}
	if _, err := app.NewSourceService(store, objects, time.Now).Write(t.Context(), actor, input); err == nil || objects.puts != 0 || store.begins != 0 || store.finishes != 0 {
		t.Fatal("partial chapter import", err, objects.puts, store.begins)
	}
}

func TestScriptUnknownObjectMismatchNeverOverwritesOrPublishes(t *testing.T) {
	actor, input := sourceCommand()
	store := &sourcePersistence{}
	objects := &sourceObjects{data: make(map[string][]byte), uncertain: true}
	service := app.NewSourceService(store, objects, time.Now)
	if _, err := service.Write(t.Context(), actor, input); !errors.Is(err, app.ErrNeedsReconciliation) {
		t.Fatal(err)
	}
	key := store.pending.Plan.Objects[0].Key
	objects.data[key] = []byte("foreign digest")
	objects.uncertain = false
	if _, err := service.Write(t.Context(), actor, input); !errors.Is(err, app.ErrObjectMismatch) || store.finishes != 0 || string(objects.data[key]) != "foreign digest" {
		t.Fatal("mismatch overwritten", err)
	}
}

func (s *sourcePersistence) ClaimSourceIO(context.Context, identityapp.Principal, app.WritePlan, uuid.UUID, time.Time) error {
	return nil
}
func (s *sourcePersistence) BeginSourceObject(context.Context, identityapp.Principal, app.WritePlan, uuid.UUID, domain.ObjectFact) error {
	return nil
}
func (s *sourcePersistence) ConfirmSourceObject(context.Context, identityapp.Principal, app.WritePlan, uuid.UUID, domain.ObjectFact) error {
	return nil
}
func (s *sourcePersistence) EndSourceIO(context.Context, app.WritePlan, uuid.UUID, bool, time.Time) error {
	return nil
}
