package aigateway_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	media "github.com/StephenQiu30/glowloom-server/backend/media/application"
	tool "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/application"
	transcript "github.com/StephenQiu30/glowloom-server/backend/media/mediatool/domain"
)

type transcriptFunc func(context.Context, *media.Downloaded, string, int64) (transcript.Transcript, error)

func (f transcriptFunc) Transcribe(ctx context.Context, file *media.Downloaded, language string, duration int64) (transcript.Transcript, error) {
	return f(ctx, file, language, duration)
}

func TestLocalTranscriptionUsesSingleZeroCreditLedgerAndWorkerProof(t *testing.T) {
	for _, noSpeech := range []bool{false, true} {
		t.Run(map[bool]string{false: "transcript", true: "no-speech"}[noSpeech], func(t *testing.T) {
			db, _, _, a := fixture(t)
			require.NoError(t, db.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=clock_timestamp() WHERE id=?`, a.ID.String()+"@fixture.invalid", a.ID).Error)
			pid, job, worker := uuid.New(), uuid.New(), uuid.New()
			require.NoError(t, db.Exec(`INSERT INTO workspace.project(id,org_id,owner_id,owner_scope_id,name,aspect_ratio,style_type) VALUES(?,?,?,?,'Local inference','16:9','realistic')`, pid, a.OrgID, a.ID, a.ScopeID).Error)
			require.NoError(t, db.Exec(`INSERT INTO mediatool.transcription_job(id,project_id,org_id,actor_id,actor_role,canvas_id,node_id,source_revision,language,frozen,status,stage,progress,attempt,revision,inference_state,active_worker,created_at,updated_at) VALUES(?,?,?,?,'producer',?,?,1,'zh','{}','running','inference',50,1,1,'submitted',?,clock_timestamp(),clock_timestamp())`, job, pid, a.OrgID, a.ID, uuid.New(), uuid.New(), worker).Error)
			f, err := os.CreateTemp(t.TempDir(), "audio-")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, f.Close()) })
			file := &media.Downloaded{File: f, SHA256: strings.Repeat("a", 64)}
			var sends atomic.Int64
			native := transcriptFunc(func(_ context.Context, _ *media.Downloaded, lang string, ms int64) (transcript.Transcript, error) {
				sends.Add(1)
				require.Equal(t, "zh", lang)
				require.EqualValues(t, 1000, ms)
				if noSpeech {
					return transcript.Transcript{}, tool.ErrNoSpeech
				}
				return transcript.Transcript{Version: 1, Language: "zh", DurationMS: 1000, Segments: []transcript.SubtitleSegment{{StartMS: 0, EndMS: 1000, Text: "已识别"}}}, nil
			})
			tracked := aigateway.TrackTranscriber(db, native).(tool.WorkTranscriber)
			id := tool.TranscriptionWorkID{JobID: job, Attempt: 1, ExecutionID: worker}
			work := tool.TranscriptionWork{Actor: a, Job: transcript.TranscriptionJob{ID: job, ProjectID: pid, Attempt: 1}}
			for range 2 {
				result, err := tracked.TranscribeWork(t.Context(), id, work, file, "zh", 1000)
				if noSpeech {
					require.ErrorIs(t, err, tool.ErrNoSpeech)
				} else {
					require.NoError(t, err)
					require.NoError(t, result.Validate())
				}
			}
			require.EqualValues(t, 1, sends.Load())
			var facts struct{ Requests, Holds, Freezes, Settlements int64 }
			require.NoError(t, db.Raw(`SELECT (SELECT count(*) FROM ai.request WHERE scope_id=? AND origin='transcription') AS requests,(SELECT count(*) FROM ai.hold h JOIN ai.request r ON r.id=h.request_id WHERE r.scope_id=? AND h.amount=0) AS holds,(SELECT count(*) FROM ai.money_event WHERE scope_id=? AND kind='freeze' AND held_delta=0) AS freezes,(SELECT count(*) FROM ai.money_event WHERE scope_id=? AND kind='settle' AND spent_delta=0) AS settlements`, a.ScopeID, a.ScopeID, a.ScopeID, a.ScopeID).Scan(&facts).Error)
			require.EqualValues(t, 1, facts.Requests)
			require.EqualValues(t, 1, facts.Holds)
			require.EqualValues(t, 1, facts.Freezes)
			require.EqualValues(t, 1, facts.Settlements)
			id.ExecutionID = uuid.New()
			_, err = tracked.TranscribeWork(t.Context(), id, work, file, "zh", 1000)
			require.True(t, errors.Is(err, tool.ErrInferenceUncertain))
			require.EqualValues(t, 1, sends.Load())
		})
	}
}
