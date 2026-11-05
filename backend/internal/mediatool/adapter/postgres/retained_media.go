package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// RetainedProjectAccess is the authorization port consumed by this read-only owner.
type RetainedProjectAccess interface {
	AuthorizeProject(context.Context, identityapp.Principal, uuid.UUID, bool) error
}

// RetainedMediaReader includes immutable sources and unpublished output intents.
// Its caller owns the transaction and keeps current project authority locked.
type RetainedMediaReader struct {
	tx     *gorm.DB
	access RetainedProjectAccess
}

// NewRetainedMediaReader binds owning history reads to current caller authority.
func NewRetainedMediaReader(tx *gorm.DB, access RetainedProjectAccess) *RetainedMediaReader {
	return &RetainedMediaReader{tx: tx, access: access}
}

// HasMediaReferences includes old sources and every retained output attempt.
func (r *RetainedMediaReader) HasMediaReferences(ctx context.Context, actor identityapp.Principal, project, asset uuid.UUID) (bool, error) {
	if asset == uuid.Nil {
		return false, application.ErrUnavailable
	}
	facts, err := r.read(ctx, actor, project)
	if err != nil {
		return false, err
	}
	_, present := facts.assets[asset]
	return present, nil
}

const retainedMediaLimit = 50000

type retainedMediaFacts struct {
	assets map[uuid.UUID]struct{}
	keys   map[string]struct{}
	rows   int
	bytes  int
}

func (f *retainedMediaFacts) body(raw []byte, out any) error {
	f.rows++
	f.bytes += len(raw)
	if f.rows > retainedMediaLimit || f.bytes > 64<<20 || len(raw) == 0 || len(raw) > 1<<20 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return application.ErrUnavailable
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil || d.Decode(new(any)) != io.EOF {
		return application.ErrUnavailable
	}
	return nil
}

func (f *retainedMediaFacts) key(key string) error {
	if len(key) == 0 || len(key) > 1024 || strings.ContainsAny(key, "\\") || strings.HasPrefix(key, "/") || path.Clean(key) != key {
		return application.ErrUnavailable
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return application.ErrUnavailable
		}
	}
	for _, c := range key {
		if unicode.IsControl(c) {
			return application.ErrUnavailable
		}
	}
	f.keys[key] = struct{}{}
	if len(f.keys) > retainedMediaLimit {
		return application.ErrUnavailable
	}
	return nil
}

func (f *retainedMediaFacts) source(source domain.FrozenSource) error {
	if source.AssetID == uuid.Nil || source.Revision < 1 || source.ByteSize < 1 || !domain.ValidDepthSHA(source.SHA256) {
		return application.ErrUnavailable
	}
	switch source.Kind {
	case "image", "video", "audio":
		if !strings.HasPrefix(source.MIMEType, source.Kind+"/") {
			return application.ErrUnavailable
		}
	default:
		return application.ErrUnavailable
	}
	f.assets[source.AssetID] = struct{}{}
	return f.key(source.ObjectKey)
}

func (r *RetainedMediaReader) read(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (retainedMediaFacts, error) {
	facts := retainedMediaFacts{assets: make(map[uuid.UUID]struct{}), keys: make(map[string]struct{})}
	if r == nil || r.tx == nil || r.tx.Statement == nil || r.access == nil || project == uuid.Nil {
		return retainedMediaFacts{}, application.ErrUnavailable
	}
	if _, ok := r.tx.Statement.ConnPool.(gorm.TxCommitter); !ok {
		return retainedMediaFacts{}, application.ErrUnavailable
	}
	if err := r.access.AuthorizeProject(ctx, actor, project, false); err != nil {
		return retainedMediaFacts{}, err
	}
	tx := r.tx.WithContext(ctx)
	// Reject excessive retained facts before reading payloads. Every subsequent
	// query also caps each payload and streams rows through the aggregate budget,
	// so concurrent history publication cannot bypass the allocation bound.
	var budget retainedPayloadBudget
	budgetQueries := []struct {
		query *gorm.DB
		field string
	}{
		{tx.Model(&jobRow{}).Where("org_id = ? AND project_id = ?", actor.OrgID, project), "frozen::text"},
		{tx.Model(&transcriptionRow{}).Where("org_id = ? AND project_id = ?", actor.OrgID, project), "frozen::text"},
		{tx.Model(&depthRow{}).Where("org_id = ? AND project_id = ?", actor.OrgID, project), "frozen::text"},
		{tx.Model(&depthResultIntentRecord{}).Joins("JOIN mediatool.depth_job ON mediatool.depth_job.id = mediatool.depth_result_intent.job_id").Where("mediatool.depth_job.org_id = ? AND mediatool.depth_job.project_id = ?", actor.OrgID, project), "mediatool.depth_result_intent.artifact::text"},
	}
	for _, item := range budgetQueries {
		part, err := readRetainedPayloadBudget(item.query, item.field)
		if err != nil {
			return retainedMediaFacts{}, fmt.Errorf("bound retained media history: %w", err)
		}
		budget.Rows += part.Rows
		budget.Bytes += part.Bytes
		if part.Largest > budget.Largest {
			budget.Largest = part.Largest
		}
	}
	if budget.Rows > retainedMediaLimit || budget.Bytes > 64<<20 || budget.Largest > 1<<20 {
		return retainedMediaFacts{}, application.ErrUnavailable
	}
	if err := readRetainedExports(tx, actor, project, &facts); err != nil {
		return retainedMediaFacts{}, fmt.Errorf("read retained export media: %w", err)
	}
	if err := eachRetainedRow(tx, tx.Model(&transcriptionRow{}).
		Select("CASE WHEN octet_length(frozen::text) <= ? THEN frozen ELSE NULL END AS frozen, language", 1<<20).
		Where("org_id = ? AND project_id = ?", actor.OrgID, project).Order("id").Limit(retainedMediaLimit+1), func(row transcriptionRow) error {
		var frozen domain.FrozenTranscription
		if facts.body(row.Frozen, &frozen) != nil || frozen.Language != row.Language || !domain.ValidTranscriptionLanguage(frozen.Language) || (frozen.Input.Kind != "audio" && frozen.Input.Kind != "video") || frozen.Input.ByteSize > 500<<20 || frozen.Input.DurationMS == nil || *frozen.Input.DurationMS < 1 || *frozen.Input.DurationMS > 86400000 {
			return application.ErrUnavailable
		}
		return facts.source(frozen.Input)
	}); err != nil {
		return retainedMediaFacts{}, fmt.Errorf("read retained transcription media: %w", err)
	}
	if err := readRetainedDepth(tx, actor, project, &facts); err != nil {
		return retainedMediaFacts{}, fmt.Errorf("read retained depth media: %w", err)
	}
	return facts, nil
}

func readRetainedExports(tx *gorm.DB, actor identityapp.Principal, project uuid.UUID, facts *retainedMediaFacts) error {
	return eachRetainedRow(tx, tx.Model(&jobRow{}).
		Select("id, attempt, created_at, output_kind, asset_id, artifact_id, CASE WHEN octet_length(frozen::text) <= ? THEN frozen ELSE NULL END AS frozen", 1<<20).
		Where("org_id = ? AND project_id = ?", actor.OrgID, project).Order("id").Limit(retainedMediaLimit+1), func(row jobRow) error {
		var frozen domain.FrozenExport
		if facts.body(row.Frozen, &frozen) != nil || row.ID == uuid.Nil || row.Attempt < 1 || row.Attempt > 100 || row.CreatedAt.IsZero() {
			return application.ErrUnavailable
		}
		kind := domain.OutputKind(row.OutputKind).Effective()
		if !kind.Valid() || frozen.OutputKind.Effective() != kind || kind == domain.OutputAudio && !application.HasAudibleClip(frozen.Timeline) {
			return application.ErrUnavailable
		}
		ids, err := application.SourceIDs(frozen.Timeline)
		if err != nil || len(ids) != len(frozen.Inputs) {
			return application.ErrUnavailable
		}
		assets := make([]mediadomain.MediaAsset, 0, len(frozen.Inputs))
		for i, source := range frozen.Inputs {
			if source.AssetID != ids[i] || facts.source(source) != nil {
				return application.ErrUnavailable
			}
			// FrozenSource predates any current catalog visibility. This local
			// projection validates the retained input geometry without rereading
			// mutable media state or asserting that the source is still available.
			assets = append(assets, mediadomain.MediaAsset{ID: source.AssetID, ProjectID: project, Kind: mediadomain.Kind(source.Kind), Origin: mediadomain.OriginSystem, Status: mediadomain.StatusReady, ModerationStatus: mediadomain.ModerationPassed, ObjectKey: source.ObjectKey, FileName: path.Base(source.ObjectKey), MimeType: source.MIMEType, ByteSize: source.ByteSize, SHA256: &source.SHA256, Revision: source.Revision, Width: source.Width, Height: source.Height, DurationMS: source.DurationMS, FPS: source.FPS, AudioChannels: source.AudioChannels, CreateTime: row.CreatedAt, UpdateTime: row.CreatedAt})
		}
		if _, err := application.FreezeInputs(frozen.Timeline, assets); err != nil {
			return application.ErrUnavailable
		}
		if kind.IsArtifact() {
			if application.ValidateProjectExport(frozen) != nil || row.AssetID != nil {
				return application.ErrUnavailable
			}
			own := row.ArtifactID == nil
			for attempt := 1; attempt <= row.Attempt; attempt++ {
				id := uuid.NewSHA1(row.ID, []byte("export-artifact/"+strconv.Itoa(attempt)))
				if row.ArtifactID != nil && *row.ArtifactID == id {
					own = true
				}
				key := path.Join("projects", project.String(), "exports", row.CreatedAt.UTC().Format("2006/01"), id.String()+".zip")
				if facts.key(key) != nil {
					return application.ErrUnavailable
				}
			}
			if !own {
				return application.ErrUnavailable
			}
			return nil
		}
		mediaKind, ext := "video", ".mp4"
		if kind == domain.OutputAudio {
			mediaKind, ext = "audio", ".m4a"
		}
		ownOutput := row.AssetID == nil
		for attempt := 1; attempt <= row.Attempt; attempt++ {
			id := uuid.NewSHA1(row.ID, []byte("export-output/"+strconv.Itoa(attempt)))
			if row.AssetID != nil && *row.AssetID == id {
				ownOutput = true
			}
			facts.assets[id] = struct{}{}
			key := path.Join("projects", project.String(), mediaKind, row.CreatedAt.UTC().Format("2006/01"), id.String()+ext)
			if facts.key(key) != nil {
				return application.ErrUnavailable
			}
			base := strings.TrimSuffix(key, ext)
			if kind == domain.OutputAudio {
				if facts.key(path.Join(base, "waveform.png")) != nil {
					return application.ErrUnavailable
				}
			} else if facts.key(path.Join(base, "poster.png")) != nil || facts.key(path.Join(base, "proxy_720p.mp4")) != nil {
				return application.ErrUnavailable
			}
		}
		if !ownOutput {
			return application.ErrUnavailable
		}
		return nil
	})
}

type retainedPayloadBudget struct {
	Rows, Bytes, Largest int64
}

func readRetainedPayloadBudget(query *gorm.DB, field string) (retainedPayloadBudget, error) {
	var budget retainedPayloadBudget
	selectExpr := fmt.Sprintf("count(*) AS rows, COALESCE(sum(octet_length(%s)), 0) AS bytes, COALESCE(max(octet_length(%s)), 0) AS largest", field, field)
	err := query.Select(selectExpr).Scan(&budget).Error
	return budget, err
}

// eachRetainedRow never accumulates unvalidated payloads in a result slice.
// Callbacks consume immutable owning facts and do not query the same connection.
func eachRetainedRow[T any](tx *gorm.DB, query *gorm.DB, consume func(T) error) error {
	rows, err := query.Rows()
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var row T
		if err := tx.ScanRows(rows, &row); err != nil {
			return err
		}
		if err := consume(row); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return rows.Close()
}
