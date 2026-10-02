package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

type objectBytes struct {
	fact domain.ObjectFact
	data []byte
}

func validateSourceCommand(input SourceCommand) error {
	if input.ProjectID == uuid.Nil || input.Key == uuid.Nil || input.RequestID == uuid.Nil || input.ExpectedRevision < 0 || input.BaseVersionID != nil && *input.BaseVersionID == uuid.Nil {
		return domain.ErrInvalidSource
	}
	switch input.Action {
	case "create", "update":
		if len(input.Sources) != 1 || !input.RightsConfirmed || len(input.Order) != 0 {
			return domain.ErrInvalidSource
		}
		if input.Action == "update" && (input.LineageID == nil || *input.LineageID == uuid.Nil) {
			return domain.ErrInvalidSource
		}
		if input.Action == "create" && input.LineageID != nil {
			return domain.ErrInvalidSource
		}
	case "import":
		if len(input.Sources) < 1 || len(input.Sources) > domain.MaxChapterImport || !input.RightsConfirmed || len(input.Order) != 0 || input.LineageID != nil {
			return domain.ErrInvalidSource
		}
	case "delete":
		if len(input.Sources) != 0 || len(input.Order) != 0 || input.LineageID == nil || *input.LineageID == uuid.Nil {
			return domain.ErrInvalidSource
		}
	case "reorder":
		if len(input.Sources) != 0 || input.LineageID != nil {
			return domain.ErrInvalidSource
		}
	default:
		return domain.ErrInvalidSource
	}
	total := 0
	for _, source := range input.Sources {
		if source.Provenance.ExternalID != nil && *source.Provenance.ExternalID == uuid.Nil || source.Provenance.ExternalPosition != nil && *source.Provenance.ExternalPosition < 0 || len(source.Provenance.Mapping) != 0 || source.Provenance.Encoding != "" || len(source.Provenance.Warnings) != 0 {
			return domain.ErrInvalidSource
		}
		canonical, err := source.Document.CanonicalJSON()
		if err != nil {
			return err
		}
		total += len(canonical)
		if total > domain.MaxVersionBytes {
			return domain.ErrInvalidDocument
		}
		if source.OriginalHTML != nil && (!utf8.ValidString(*source.OriginalHTML) || strings.ContainsRune(*source.OriginalHTML, 0) || len(*source.OriginalHTML) > domain.MaxDocumentBytes) {
			return domain.ErrInvalidDocument
		}
		record := domain.SourceDocument{ID: input.Key, LineageID: input.Key, Revision: 1, Kind: source.Kind, Title: source.Title, Status: source.Status, Document: source.Document}
		if err := record.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Write publishes a complete immutable draft only after every private byte proof.
// Uncertain object or commit outcomes retain the original plan and request key.
func (s *SourceService) Write(ctx context.Context, actor identityapp.Principal, input SourceCommand) (SourceReceipt, error) {
	if s == nil || s.store == nil || s.objects == nil || s.now == nil {
		return SourceReceipt{}, ErrUnavailable
	}
	if err := validateSourceCommand(input); err != nil {
		return SourceReceipt{}, err
	}
	hash, err := commandHash(input)
	if err != nil {
		return SourceReceipt{}, err
	}
	pending, err := s.store.FindCommand(ctx, actor, input.ProjectID, input.Key, hash)
	if err != nil {
		return SourceReceipt{}, err
	}
	if pending != nil && pending.Receipt != nil {
		return *pending.Receipt, nil
	}
	base, err := s.store.LoadBase(ctx, actor, input.ProjectID, input.BaseVersionID)
	if err != nil {
		return SourceReceipt{}, err
	}
	at := s.now().UTC().Truncate(time.Microsecond)
	if pending != nil {
		at = pending.Plan.CreatedAt
	}
	plan, objects, err := s.prepareWrite(ctx, actor, input, hash, base, at)
	if err != nil {
		return SourceReceipt{}, err
	}
	return s.writePrepared(ctx, actor, plan, objects)
}

func (s *SourceService) writePrepared(ctx context.Context, actor identityapp.Principal, plan WritePlan, objects []objectBytes) (SourceReceipt, error) {
	staged, err := s.store.BeginWrite(ctx, actor, plan)
	if err != nil {
		return SourceReceipt{}, err
	}
	if staged.Receipt != nil {
		return *staged.Receipt, nil
	}
	if staged.Plan.Reuse {
		if len(plan.NewSources) != 0 || plan.Version.SourceManifestSHA256 != staged.Plan.Version.SourceManifestSHA256 {
			return SourceReceipt{}, ErrIdempotencyConflict
		}
		return s.store.FinishWrite(ctx, actor, staged.Plan)
	}
	if !slices.Equal(plan.Objects, staged.Plan.Objects) || plan.Version.SourceManifestSHA256 != staged.Plan.Version.SourceManifestSHA256 {
		return SourceReceipt{}, ErrIdempotencyConflict
	}
	owner := uuid.New()
	ioCtx, entry := s.io.begin(ctx, owner)
	if err := s.store.ClaimSourceIO(ctx, actor, staged.Plan, owner, s.now().UTC()); err != nil {
		s.io.end(owner, entry, true)
		return SourceReceipt{}, err
	}
	complete := false
	defer func() {
		exitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		err := s.store.EndSourceIO(exitCtx, staged.Plan, owner, !complete, s.now().UTC())
		s.io.end(owner, entry, err == nil)
	}()
	for _, object := range objects {
		if err := s.store.BeginSourceObject(ioCtx, actor, staged.Plan, owner, object.fact); err != nil {
			return SourceReceipt{}, err
		}
		if err := s.ensureObject(ioCtx, object); err != nil {
			return SourceReceipt{}, fmt.Errorf("store frozen script object: %w", errors.Join(ErrNeedsReconciliation, err))
		}
		if err := s.store.ConfirmSourceObject(ioCtx, actor, staged.Plan, owner, object.fact); err != nil {
			return SourceReceipt{}, err
		}
	}
	result, err := s.store.FinishWrite(ioCtx, actor, staged.Plan)
	if err != nil {
		return SourceReceipt{}, fmt.Errorf("commit immutable script version: %w", err)
	}
	complete = true
	return result, nil
}

func (s *SourceService) readObject(ctx context.Context, fact domain.ObjectFact) ([]byte, error) {
	if err := fact.Validate(); err != nil {
		return nil, err
	}
	reader, err := s.objects.Get(ctx, fact.Key)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return nil, ErrUnavailable
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, fact.ByteSize+1))
	closeErr := reader.Close()
	if err := errors.Join(readErr, closeErr, ctx.Err()); err != nil {
		return nil, err
	}
	if int64(len(data)) != fact.ByteSize || domain.ContentSHA(data) != fact.SHA256 {
		return nil, ErrObjectMismatch
	}
	return data, nil
}

func (s *SourceService) ensureObject(ctx context.Context, object objectBytes) error {
	if _, err := s.readObject(ctx, object.fact); err == nil {
		return nil
	} else if !errors.Is(err, ErrObjectMissing) {
		return err
	}
	if err := s.objects.PutIfAbsent(ctx, object.fact.Key, bytes.NewReader(object.data), object.fact.ByteSize, object.fact.MIME, object.fact.SHA256); err != nil && !errors.Is(err, ErrObjectExists) {
		return err
	}
	_, err := s.readObject(ctx, object.fact)
	return err
}

func frozenObject(project, id uuid.UUID, name, mime string, data []byte) domain.ObjectFact {
	return domain.ObjectFact{Key: "projects/" + project.String() + "/script/" + id.String() + "/" + name, SHA256: domain.ContentSHA(data), ByteSize: int64(len(data)), MIME: mime}
}

func (s *SourceService) prepareWrite(ctx context.Context, actor identityapp.Principal, input SourceCommand, hash string, base SourceBase, now time.Time) (WritePlan, []objectBytes, error) {
	plan := WritePlan{Command: input, RequestHash: hash, ActorID: actor.ID, OrgID: actor.OrgID, CreatedAt: now, NewSources: make([]domain.SourceRecord, 0), Mappings: make([]SourceChange, 0), Objects: make([]domain.ObjectFact, 0)}
	plan.Command.Sources = nil
	records := slices.Clone(base.Sources)
	documents := make([]domain.SourceDocument, 0, len(records)+len(input.Sources))
	for _, record := range records {
		data, err := s.readObject(ctx, record.Rich)
		if err != nil {
			return WritePlan{}, nil, fmt.Errorf("read original rich snapshot: %w", err)
		}
		doc, err := domain.DecodeRichDocument(data)
		if err != nil {
			return WritePlan{}, nil, err
		}
		documents = append(documents, sourceDocument(record, doc))
	}
	objects := make([]objectBytes, 0, len(input.Sources)*2+2)
	switch input.Action {
	case "create", "import":
		for i, source := range input.Sources {
			id := uuid.NewSHA1(input.Key, []byte("source/"+actor.ID.String()+"/"+input.ProjectID.String()+fmt.Sprint(i)))
			record, doc, raw, err := makeSource(input.ProjectID, actor, id, id, nil, 1, source, now)
			if err != nil {
				return WritePlan{}, nil, err
			}
			records = append(records, record)
			documents = append(documents, doc)
			plan.NewSources = append(plan.NewSources, record)
			objects = append(objects, raw...)
			plan.Mappings = append(plan.Mappings, SourceChange{LineageID: id, NewSourceID: &id})
		}
	case "update", "delete":
		index := slices.IndexFunc(records, func(r domain.SourceRecord) bool { return r.LineageID == *input.LineageID })
		if index < 0 {
			return WritePlan{}, nil, ErrNotFound
		}
		old := records[index]
		if input.Action == "delete" {
			records = slices.Delete(records, index, index+1)
			documents = slices.Delete(documents, index, index+1)
			plan.Mappings = append(plan.Mappings, SourceChange{LineageID: old.LineageID, OldSourceID: &old.ID})
		} else {
			id := uuid.NewSHA1(input.Key, []byte("source/"+actor.ID.String()+"/"+input.ProjectID.String()+"/"+old.LineageID.String()))
			record, doc, raw, err := makeSource(input.ProjectID, actor, id, old.LineageID, &old.ID, old.Revision+1, input.Sources[0], now)
			if err != nil {
				return WritePlan{}, nil, err
			}
			records[index], documents[index] = record, doc
			plan.NewSources = append(plan.NewSources, record)
			objects = append(objects, raw...)
			plan.Mappings = append(plan.Mappings, SourceChange{LineageID: old.LineageID, OldSourceID: &old.ID, NewSourceID: &id})
		}
	case "reorder":
		if len(input.Order) != len(records) {
			return WritePlan{}, nil, domain.ErrInvalidSource
		}
		nextRecords := make([]domain.SourceRecord, 0, len(records))
		nextDocs := make([]domain.SourceDocument, 0, len(records))
		seen := make(map[uuid.UUID]bool, len(records))
		for _, lineage := range input.Order {
			if lineage == uuid.Nil || seen[lineage] {
				return WritePlan{}, nil, domain.ErrInvalidSource
			}
			seen[lineage] = true
			index := slices.IndexFunc(records, func(r domain.SourceRecord) bool { return r.LineageID == lineage })
			if index < 0 {
				return WritePlan{}, nil, domain.ErrInvalidSource
			}
			nextRecords = append(nextRecords, records[index])
			nextDocs = append(nextDocs, documents[index])
		}
		records, documents = nextRecords, nextDocs
	}
	composed, err := domain.ComposeSources(documents)
	if err != nil {
		return WritePlan{}, nil, err
	}
	manifest, err := json.Marshal(records)
	if err != nil {
		return WritePlan{}, nil, err
	}
	if len(manifest) > domain.MaxHTTPBytes {
		return WritePlan{}, nil, domain.ErrInvalidDocument
	}
	versionID := uuid.NewSHA1(input.Key, []byte("version/"+actor.ID.String()+"/"+input.ProjectID.String()))
	text := frozenObject(input.ProjectID, versionID, "text.txt", "text/plain; charset=utf-8", []byte(composed.Text))
	rich := frozenObject(input.ProjectID, versionID, "document.json", "application/json", composed.RichJSON)
	ids := make([]uuid.UUID, len(records))
	for i, r := range records {
		ids[i] = r.ID
	}
	versionNo := int64(1)
	if base.Version != nil {
		versionNo = base.Version.VersionNo + 1
	}
	plan.Version = domain.ScriptVersion{ID: versionID, OrgID: actor.OrgID, ProjectID: input.ProjectID, VersionNo: versionNo, SourceIDs: ids, Spans: composed.Spans, Text: text, Rich: rich, ContentHash: composed.ContentHash, DocumentSHA256: composed.DocumentSHA256, SourceManifestSHA256: domain.ContentSHA(manifest), CharCount: composed.CharCount, CreatedAt: now}
	plan.Candidate = domain.SplitSet{ID: uuid.NewSHA1(versionID, []byte("source-candidates")), VersionID: versionID, OrgID: actor.OrgID, ProjectID: input.ProjectID, Kind: "candidate", Origin: "sources", Boundaries: composed.Candidates, CreatedAt: now}
	objects = append(objects, objectBytes{text, []byte(composed.Text)}, objectBytes{rich, composed.RichJSON})
	for _, object := range objects {
		plan.Objects = append(plan.Objects, object.fact)
	}
	return plan, objects, nil
}

func sourceDocument(r domain.SourceRecord, d domain.RichDocument) domain.SourceDocument {
	return domain.SourceDocument{ID: r.ID, LineageID: r.LineageID, PreviousID: r.PreviousID, Revision: r.Revision, Kind: r.Kind, Title: r.Title, Status: r.Status, Document: d}
}

func makeSource(project uuid.UUID, actor identityapp.Principal, id, lineage uuid.UUID, previous *uuid.UUID, revision int64, input SourceInput, now time.Time) (domain.SourceRecord, domain.SourceDocument, []objectBytes, error) {
	canonical, err := input.Document.CanonicalJSON()
	if err != nil {
		return domain.SourceRecord{}, domain.SourceDocument{}, nil, err
	}
	doc, err := domain.DecodeRichDocument(canonical)
	if err != nil {
		return domain.SourceRecord{}, domain.SourceDocument{}, nil, err
	}
	plain, err := doc.PlainText()
	if err != nil {
		return domain.SourceRecord{}, domain.SourceDocument{}, nil, err
	}
	original, mime := canonical, "application/json"
	origin := "manual"
	if input.OriginalHTML != nil {
		original, mime = []byte(*input.OriginalHTML), "text/html; charset=utf-8"
		origin = "beeftv"
	}
	record := domain.SourceRecord{ID: id, OrgID: actor.OrgID, ProjectID: project, LineageID: lineage, PreviousID: previous, Revision: revision, Origin: origin, Kind: input.Kind, Title: input.Title, Status: input.Status, RightsActorID: actor.ID, RightsConfirmedAt: now, Original: frozenObject(project, id, "original", mime, original), Rich: frozenObject(project, id, "rich.json", "application/json", canonical), ContentHash: domain.ContentSHA([]byte(plain)), CharCount: utf8.RuneCountInString(plain), Provenance: input.Provenance, CreatedAt: now}
	return record, sourceDocument(record, doc), []objectBytes{{record.Original, original}, {record.Rich, canonical}}, nil
}
