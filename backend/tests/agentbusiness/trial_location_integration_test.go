package agentbusiness_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/trialtask"
	operation "github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

func TestTrialRealPGFreezesOriginalOutputBeforeQueueAndNeverRepinsReplay(t *testing.T) {
	db, _, _, actor, _ := realCanvasBoundary(t)
	selected := "trial-first"
	admissions := 0
	client := trialtask.NewSessions(db).WithOutputAdmission(func(_ context.Context, tx *gorm.DB, in operation.TrialInput) (mediadomain.ObjectRef, error) {
		admissions++
		var n int64
		if err := tx.Raw(`SELECT count(*) FROM operation.development_trial WHERE id=?`, operation.TrialID(in.Actor, in.Request.RequestID)).Scan(&n).Error; err != nil || n != 0 {
			t.Fatal("output pinned after dispatch admission", n, err)
		}
		return mediadomain.ObjectRef{ProfileID: selected, Bucket: "trial-bucket", Key: operation.TrialObjectKey(in, 1)}, nil
	})
	input := operation.TrialInput{Actor: actor, Request: operation.TrialRequest{RequestID: uuid.New(), Source: "codex", Kind: "image", Model: "fixture-image", Prompt: "circle"}, OutputRef: &mediadomain.ObjectRef{ProfileID: "forged", Bucket: "forged", Key: "forged"}}
	first, err := client.Start(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	var row struct{ Input []byte }
	if err := db.Raw(`SELECT input FROM operation.development_trial WHERE id=?`, operation.TrialID(actor, input.Request.RequestID)).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	var saved operation.TrialInput
	if json.Unmarshal(row.Input, &saved) != nil || saved.OutputRef == nil || saved.OutputRef.ProfileID != "trial-first" {
		t.Fatal("untrusted or missing original route", string(row.Input))
	}
	selected = "trial-second"
	replay, err := client.Start(t.Context(), input)
	if err != nil || replay.Fingerprint != first.Fingerprint || admissions != 1 {
		t.Fatal("original replay repinned", admissions, err)
	}
	input.Request.Prompt = "different"
	if _, err = client.Start(t.Context(), input); !errors.Is(err, trialtask.ErrConflict) {
		t.Fatal("same key changed trial", err)
	}
	input.Request.RequestID = uuid.New()
	if _, err = client.Start(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	if admissions != 2 {
		t.Fatal(admissions)
	}
	if err := db.Raw(`SELECT input FROM operation.development_trial WHERE id=?`, operation.TrialID(actor, input.Request.RequestID)).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(row.Input, &saved) != nil || saved.OutputRef == nil || saved.OutputRef.ProfileID != "trial-second" {
		t.Fatal("new intent used old target", string(row.Input))
	}
}
