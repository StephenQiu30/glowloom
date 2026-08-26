package application

import (
	"context"
	"errors"
	"time"

	eventingapp "github.com/StephenQiu30/lanverse/backend/internal/eventing/application"
	eventing "github.com/StephenQiu30/lanverse/backend/internal/eventing/domain"
	search "github.com/StephenQiu30/lanverse/backend/internal/search/domain"
)

const projectionTimeout = 30 * time.Second

type Projector struct {
	snapshots SnapshotReader
	index     Index
	now       func() time.Time
}

func NewProjector(snapshots SnapshotReader, index Index, now func() time.Time) *Projector {
	return &Projector{snapshots: snapshots, index: index, now: now}
}

func (projector *Projector) Process(ctx context.Context, envelope eventing.Envelope) error {
	if projector == nil || projector.snapshots == nil || projector.index == nil || projector.now == nil {
		return errors.New("search projector configuration is invalid")
	}
	var snapshot search.Snapshot
	var err error
	switch envelope.EventType {
	case eventing.ScriptVersionPublished:
		snapshot, err = projector.snapshots.CurrentScriptSnapshot(ctx, envelope.ProjectID)
	case eventing.StoryGraphVersionPublished:
		snapshot, err = projector.snapshots.CurrentStoryGraphSnapshot(ctx, envelope.ProjectID)
	default:
		return eventingapp.Permanent(errors.New("event type has no registered search projection"))
	}
	if err != nil {
		return err
	}
	projectionContext, cancelProjection := context.WithTimeout(ctx, projectionTimeout)
	defer cancelProjection()
	return projector.index.Project(projectionContext, snapshot, search.ProjectionSource{Kind: search.SourceEvent, ID: envelope.EventID}, projector.now().UTC())
}

type Reindexer struct {
	snapshots SnapshotReader
	index     Index
	now       func() time.Time
	newID     func() string
}

func NewReindexer(snapshots SnapshotReader, index Index, now func() time.Time, newID func() string) *Reindexer {
	return &Reindexer{snapshots: snapshots, index: index, now: now, newID: newID}
}

func (service *Reindexer) Reindex(ctx context.Context, kind search.Kind) (search.ReindexResult, error) {
	if service == nil || service.snapshots == nil || service.index == nil || service.now == nil || service.newID == nil {
		return search.ReindexResult{}, errors.New("search reindex configuration is invalid")
	}
	var snapshots []search.Snapshot
	var err error
	switch kind {
	case search.KindScript:
		snapshots, err = service.snapshots.AllScriptSnapshots(ctx)
	case search.KindStoryGraph:
		snapshots, err = service.snapshots.AllStoryGraphSnapshots(ctx)
	default:
		return search.ReindexResult{}, errors.New("search reindex kind is invalid")
	}
	if err != nil {
		return search.ReindexResult{}, err
	}
	source := search.ProjectionSource{Kind: search.SourceReindex, ID: service.newID()}
	if err = source.Validate(); err != nil {
		return search.ReindexResult{}, err
	}
	startedAt := service.now().UTC()
	result, err := service.index.Rebuild(ctx, kind, snapshots, source, startedAt)
	if err != nil {
		return search.ReindexResult{}, err
	}
	// Reload after the atomic alias switch. A publication committed while the new
	// backing index was being built is now either consumed against the new alias
	// or included in this PostgreSQL catch-up snapshot.
	if kind == search.KindScript {
		snapshots, err = service.snapshots.AllScriptSnapshots(ctx)
	} else {
		snapshots, err = service.snapshots.AllStoryGraphSnapshots(ctx)
	}
	if err != nil {
		return search.ReindexResult{}, err
	}
	for _, snapshot := range snapshots {
		if err = service.index.Project(ctx, snapshot, source, service.now().UTC()); err != nil {
			return search.ReindexResult{}, err
		}
	}
	return result, nil
}
