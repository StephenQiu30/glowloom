package workflow_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	generationgorm "github.com/StephenQiu30/lanverse/backend/internal/generation/adapter/gormdb"
	genapp "github.com/StephenQiu30/lanverse/backend/internal/generation/application"
	gen "github.com/StephenQiu30/lanverse/backend/internal/generation/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	generationtestgorm "github.com/StephenQiu30/lanverse/backend/tests/generation/adapter/gormdb"
)

type visionReviewTrackedObjects struct {
	base       *referenceExecutionHTTPObjects
	beforeRead func(int) error
	reads      int
	buffers    [][]byte
}

func (o *visionReviewTrackedObjects) ReadVerified(ctx context.Context, key string, size int64, hash string, max int64) ([]byte, error) {
	o.reads++
	if o.beforeRead != nil {
		if err := o.beforeRead(o.reads); err != nil {
			return nil, err
		}
	}
	contents, err := o.base.ReadVerified(ctx, key, size, hash, max)
	o.buffers = append(o.buffers, contents)
	return contents, err
}

func assertPersistedVisionReviewMedia(t *testing.T, ctx context.Context, database *generationtestgorm.Database, actor genapp.Actor, execution gen.ReferenceExecution, bundles gen.ReferenceBundleInputCollection, objects *referenceExecutionHTTPObjects) {
	t.Helper()
	store := generationgorm.New(database)
	index := -1
	for i, bundle := range bundles.Bundles {
		if bundle.Admission.InternalReviewReady {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("no complete media group")
	}
	input, err := store.CompileBaseVisionReviewInput(ctx, actor, execution.ProjectID, execution.ID, index, strings.Repeat("e", 64))
	if err != nil {
		t.Fatal(err)
	}
	objects.mu.Lock()
	writes := objects.writes
	objects.mu.Unlock()
	for _, mode := range []string{"ready", "forged_input", "revoked_token", "head_drift", "object_failure"} {
		t.Run("vision_media_"+mode, func(t *testing.T) {
			tracked := &visionReviewTrackedObjects{base: objects}
			restore := func() {}
			defer func() { restore() }()
			tracked.beforeRead = func(n int) error {
				if mode == "object_failure" && n == 2 {
					return errors.New("owned fixture object unavailable")
				}
				if n != 1 {
					return nil
				}
				// A second connection can commit while object IO is in progress:
				// no facts transaction may retain its row locks across this callback.
				checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				switch mode {
				case "revoked_token":
					var account model.UserAccount
					if err := database.WithContext(checkCtx).First(&account, "id = ?", actor.UserID).Error; err != nil {
						return err
					}
					restore = func() {
						if err := database.WithContext(ctx).Model(&model.UserAccount{}).Where("id = ?", actor.UserID).UpdateColumn("token_version", account.TokenVersion).Error; err != nil {
							t.Error(err)
						}
					}
					return database.WithContext(checkCtx).Model(&model.UserAccount{}).Where("id = ?", actor.UserID).UpdateColumn("token_version", account.TokenVersion+1).Error
				case "head_drift":
					restore = func() {
						if err := database.WithContext(ctx).Model(&model.GenerationReferenceExecutionHead{}).Where("project_id = ? AND target_id = ?", execution.ProjectID, execution.ReadSet.TargetRef.ID).UpdateColumn("current_execution_hash", execution.ContentHash).Error; err != nil {
							t.Error(err)
						}
					}
					return database.WithContext(checkCtx).Model(&model.GenerationReferenceExecutionHead{}).Where("project_id = ? AND target_id = ?", execution.ProjectID, execution.ReadSet.TargetRef.ID).UpdateColumn("current_execution_hash", strings.Repeat("f", 64)).Error
				}
				return nil
			}
			reader, err := genapp.NewVisionReviewMediaReader(store, tracked, gen.ReferenceObjectStoreRef{Profile: "minio", Bucket: "lanverse"})
			if err != nil {
				t.Fatal(err)
			}
			expected := input
			if mode == "forged_input" {
				expected.VisualContext.VisualGrammar.Medium = "forged but valid grammar"
				expected, err = contract.BuildVisionReviewInput(expected)
				if err != nil {
					t.Fatal(err)
				}
			}
			media, err := reader.Load(ctx, actor, expected)
			if mode == "ready" {
				if err != nil || len(media) != len(input.Attachments) {
					t.Fatalf("load actual staged bytes: %v", err)
				}
				for i, item := range media {
					hash := sha256.Sum256(item.Contents)
					if item.Attachment != input.Attachments[i] || hex.EncodeToString(hash[:]) != input.Attachments[i].Slot.SHA256 {
						t.Fatal("media did not match frozen review slot")
					}
				}
				media.Clear()
			} else {
				if err == nil || media != nil {
					t.Fatalf("returned media after %s", mode)
				}
				if mode == "forged_input" && tracked.reads != 0 {
					t.Fatal("forged full input caused object IO")
				}
				if (mode == "revoked_token" || mode == "head_drift") && tracked.reads != len(input.Attachments) {
					t.Fatalf("post-read fence not exercised; reads=%d error=%v", tracked.reads, err)
				}
			}
			for _, buffer := range tracked.buffers {
				if !bytes.Equal(buffer, make([]byte, len(buffer))) {
					t.Fatal("private bytes retained after failure or release")
				}
			}
		})
	}
	objects.mu.Lock()
	defer objects.mu.Unlock()
	if objects.writes != writes {
		t.Fatal("review media reading wrote objects")
	}
}
