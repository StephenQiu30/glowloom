package application

import (
	"context"
	"encoding/json"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// WriteFiles is a server-only publication contract. Its store must be bound to the
// exact claimed import; the ordinary source store explicitly rejects file origins.
func (s *SourceService) WriteFiles(ctx context.Context, actor identityapp.Principal, in SourceCommand, files []PreparedImportFile, rightsAt time.Time, order []uuid.UUID) (SourceReceipt, error) {
	if s == nil || s.store == nil || s.objects == nil || s.now == nil {
		return SourceReceipt{}, ErrUnavailable
	}
	if in.Action != "import" || in.ProjectID == uuid.Nil || in.Key == uuid.Nil || in.RequestID == uuid.Nil || !in.RightsConfirmed || len(files) < 1 || len(files) > 200 || rightsAt.IsZero() {
		return SourceReceipt{}, domain.ErrInvalidSource
	}
	type frozen struct {
		ID                        uuid.UUID
		SourceSHA, RichSHA, Title string
	}
	proofs := make([]frozen, len(files))
	previous := -1
	for i, file := range files {
		if file.Frozen.Position <= previous || file.Frozen.SourceID == uuid.Nil || file.Frozen.Source.ProjectID != in.ProjectID || int64(len(file.Original)) != file.Frozen.Source.ByteSize || domain.ContentSHA(file.Original) != file.Frozen.Source.SHA256 {
			return SourceReceipt{}, domain.ErrInvalidSource
		}
		previous = file.Frozen.Position
		rich, err := file.Extracted.Document.CanonicalJSON()
		if err != nil {
			return SourceReceipt{}, err
		}
		proofs[i] = frozen{file.Frozen.SourceID, file.Frozen.Source.SHA256, domain.ContentSHA(rich), file.Title}
	}
	hashInput := in
	hashInput.RequestID = uuid.Nil
	encoded, err := json.Marshal(struct {
		Command SourceCommand
		Files   []frozen
	}{hashInput, proofs})
	if err != nil {
		return SourceReceipt{}, err
	}
	hash := domain.ContentSHA(encoded)
	pending, err := s.store.FindCommand(ctx, actor, in.ProjectID, in.Key, hash)
	if err != nil {
		return SourceReceipt{}, err
	}
	if pending != nil && pending.Receipt != nil {
		return *pending.Receipt, nil
	}
	base, err := s.store.LoadBase(ctx, actor, in.ProjectID, in.BaseVersionID)
	if err != nil {
		return SourceReceipt{}, err
	}
	at := s.now().UTC().Truncate(time.Microsecond)
	if pending != nil {
		at = pending.Plan.CreatedAt
	}
	plan := WritePlan{Command: in, RequestHash: hash, ActorID: actor.ID, OrgID: actor.OrgID, CreatedAt: at, NewSources: []domain.SourceRecord{}, Mappings: []SourceChange{}, Objects: []domain.ObjectFact{}}
	records := slices.Clone(base.Sources)
	documents := make([]domain.SourceDocument, 0, len(records)+len(files))
	for _, record := range records {
		data, err := s.readObject(ctx, record.Rich)
		if err != nil {
			return SourceReceipt{}, err
		}
		doc, err := domain.DecodeRichDocument(data)
		if err != nil {
			return SourceReceipt{}, err
		}
		documents = append(documents, sourceDocument(record, doc))
	}
	objects := []objectBytes{}
	for _, file := range files {
		id := file.Frozen.SourceID
		r, doc, raw, err := makeSource(in.ProjectID, actor, id, id, nil, 1, SourceInput{Kind: "document", Title: file.Title, Status: "draft", Document: file.Extracted.Document}, at)
		if err != nil {
			return SourceReceipt{}, err
		}
		r.Origin = "file"
		r.MediaAssetID = &file.Frozen.Source.AssetID
		r.MediaRevision = &file.Frozen.Source.Revision
		r.MediaSHA256 = &file.Frozen.Source.SHA256
		r.RightsConfirmedAt = rightsAt.UTC().Truncate(time.Microsecond)
		r.Original = frozenObject(in.ProjectID, id, "original", file.Frozen.Source.MIME, file.Original)
		r.Provenance = domain.SourceProvenance{Encoding: file.Extracted.Encoding, Mapping: file.Extracted.Mapping, Warnings: file.Extracted.Warnings}
		raw[0] = objectBytes{r.Original, file.Original}
		records = append(records, r)
		documents = append(documents, doc)
		plan.NewSources = append(plan.NewSources, r)
		objects = append(objects, raw...)
		plan.Mappings = append(plan.Mappings, SourceChange{LineageID: id, NewSourceID: &id})
	}
	idsBefore := make([]uuid.UUID, len(records))
	for i, r := range records {
		idsBefore[i] = r.ID
	}
	ordered, err := domain.OrderImportSources(idsBefore, order)
	if err != nil {
		return SourceReceipt{}, err
	}
	orderedRecords := make([]domain.SourceRecord, 0, len(records))
	orderedDocuments := make([]domain.SourceDocument, 0, len(records))
	for _, id := range ordered {
		index := slices.Index(idsBefore, id)
		orderedRecords = append(orderedRecords, records[index])
		orderedDocuments = append(orderedDocuments, documents[index])
	}
	records, documents = orderedRecords, orderedDocuments
	composed, err := domain.ComposeSources(documents)
	if err != nil {
		return SourceReceipt{}, err
	}
	manifest, err := json.Marshal(records)
	if err != nil || len(manifest) > domain.MaxHTTPBytes {
		return SourceReceipt{}, domain.ErrInvalidDocument
	}
	versionID := uuid.NewSHA1(in.Key, []byte("version/"+actor.ID.String()+"/"+in.ProjectID.String()))
	text := frozenObject(in.ProjectID, versionID, "text.txt", "text/plain; charset=utf-8", []byte(composed.Text))
	rich := frozenObject(in.ProjectID, versionID, "document.json", "application/json", composed.RichJSON)
	ids := make([]uuid.UUID, len(records))
	for i, r := range records {
		ids[i] = r.ID
	}
	number := int64(1)
	if base.Version != nil {
		number = base.Version.VersionNo + 1
	}
	plan.Version = domain.ScriptVersion{ID: versionID, OrgID: actor.OrgID, ProjectID: in.ProjectID, VersionNo: number, SourceIDs: ids, Spans: composed.Spans, Text: text, Rich: rich, ContentHash: composed.ContentHash, DocumentSHA256: composed.DocumentSHA256, SourceManifestSHA256: domain.ContentSHA(manifest), CharCount: utf8.RuneCountInString(composed.Text), CreatedAt: at}
	plan.Candidate = domain.SplitSet{ID: uuid.NewSHA1(versionID, []byte("source-candidates")), VersionID: versionID, OrgID: actor.OrgID, ProjectID: in.ProjectID, Kind: "candidate", Origin: "sources", Boundaries: composed.Candidates, CreatedAt: at}
	objects = append(objects, objectBytes{text, []byte(composed.Text)}, objectBytes{rich, composed.RichJSON})
	for _, object := range objects {
		plan.Objects = append(plan.Objects, object.fact)
	}
	return s.writePrepared(ctx, actor, plan, objects)
}
