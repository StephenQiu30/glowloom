package operation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	task "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"
)

type reviewProbe struct {
	s                *stateStore
	ingests, reviews int
	fail             bool
	status           string
}

func (p *reviewProbe) Ingest(_ context.Context, in media.IngestInput) (media.IngestOutput, error) {
	p.ingests++
	id := uuid.New()
	p.s.loaded.Outputs = append(p.s.loaded.Outputs, application.WorkflowOutput{ID: id, SeqNo: int32(in.SeqNo), ModerationStatus: "pending"})
	return media.IngestOutput{OutputID: id.String()}, nil
}
func (p *reviewProbe) RecordModeration(_ context.Context, in media.RecordModerationInput) (media.RecordModerationOutput, error) {
	p.reviews++
	if p.fail {
		return media.RecordModerationOutput{}, errors.New("review persistence interrupted")
	}
	p.status = string(in.Status)
	p.s.loaded.Outputs[0].ModerationStatus = in.Status
	return media.RecordModerationOutput{}, nil
}
func TestMediaTaskRetriesReviewWithoutRepeatingProviderOrIngest(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusIngesting
	s.loaded.Operation.Params = []byte(`{"mock_result_urls":["https://fixture/image.png"],"mock_moderation_status":"passed"}`)
	p := &reviewProbe{s: s, fail: true}
	h := map[string]taskruntime.Handler{}
	task.RegisterMedia(h, s, p, nil)
	job := taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()}
	_, err := h["operation.ingest"](t.Context(), job)
	require.Error(t, err)
	require.Equal(t, 1, p.ingests)
	p.fail = false
	_, err = h["operation.ingest"](t.Context(), job)
	require.NoError(t, err)
	_, err = h["operation.ingest"](t.Context(), job)
	require.NoError(t, err)
	require.Equal(t, 1, p.ingests)
	require.Equal(t, 2, p.reviews)
	require.Equal(t, "passed", p.status)
	require.Empty(t, s.calls)
}
func TestMediaTaskPersistsExplicitRejection(t *testing.T) {
	s := newStateStore()
	s.loaded.Operation.Status = domain.StatusIngesting
	s.loaded.Operation.Params = []byte(`{"mock_result_urls":["https://fixture/image.png"],"mock_moderation_status":"rejected"}`)
	p := &reviewProbe{s: s}
	h := map[string]taskruntime.Handler{}
	task.RegisterMedia(h, s, p, nil)
	_, err := h["operation.ingest"](t.Context(), taskruntime.Job{BusinessID: s.loaded.Operation.ID.String()})
	require.NoError(t, err)
	require.Equal(t, "rejected", p.status)
	require.Empty(t, s.settled)
}
