package media_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func TestVoiceLibraryPostgresPersistence(t *testing.T) {
	db := mediaStoreDB(t)
	actor, pid := mediaStoreProject(t, db)
	other, otherPID := mediaStoreProject(t, db)
	s := mediaTestStore(db)
	duration := int32(3000)
	now := time.Now()
	id := uuid.New()
	asset := domain.MediaAsset{ID: id, ProjectID: pid, Kind: domain.KindAudio, Origin: domain.OriginUpload, Status: domain.StatusUploading, ObjectKey: "projects/" + pid.String() + "/audio/2026/10/" + id.String() + ".wav", FileName: "voice.wav", MimeType: "audio/wav", ByteSize: 144044, DurationMS: &duration, ModerationStatus: domain.ModerationPending, Revision: 1, CreateTime: now, UpdateTime: now}
	if err := s.CreateAsset(t.Context(), actor, asset); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE media.media_asset SET status='ready',moderation_status='passed' WHERE id=?`, id).Error; err != nil {
		t.Fatal(err)
	}
	input := application.VoiceCreate{RequestID: uuid.New(), Name: "回归音色", VoiceTags: application.VoiceTags{Gender: "男声", Language: "中文", Style: "旁白"}, AssetID: id, Consent: true}
	voice, err := s.CreateVoice(t.Context(), actor, pid, input)
	if err != nil {
		t.Fatal(err)
	}
	if voice.Gender != "男声" || voice.Language != "中文" || voice.Age != "未标注" {
		t.Fatal("voice metadata not persisted", voice)
	}
	replay, err := s.CreateVoice(t.Context(), actor, pid, input)
	if err != nil || replay.ID != voice.ID {
		t.Fatal("idempotency", err)
	}
	input.Name = "冲突"
	if _, err = s.CreateVoice(t.Context(), actor, pid, input); !errors.Is(err, application.ErrVoiceConflict) {
		t.Fatal("conflicting replay", err)
	}
	if _, err = s.CreateVoice(t.Context(), other, otherPID, application.VoiceCreate{RequestID: uuid.New(), Name: "跨项目", AssetID: id, Consent: true}); err == nil {
		t.Fatal("foreign source accepted")
	}
	if _, err = s.ListVoices(t.Context(), other, pid, uuid.Nil); err == nil {
		t.Fatal("foreign owner read")
	}
	page, err := s.ListVoices(t.Context(), actor, pid, uuid.Nil)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != voice.ID {
		t.Fatal("readback", page, err)
	}
	updated, err := s.UpdateVoice(t.Context(), actor, pid, voice.ID, application.VoiceUpdate{Revision: voice.Revision, Name: "已改名", Favorite: true})
	if err != nil || !updated.Favorite || updated.Revision != 2 {
		t.Fatal("update", err)
	}
	if _, err = s.UpdateVoice(t.Context(), actor, pid, voice.ID, application.VoiceUpdate{Revision: voice.Revision, Name: "旧修订"}); !errors.Is(err, application.ErrVoiceConflict) {
		t.Fatal("stale write", err)
	}
	if err = s.FavoriteSystemVoice(t.Context(), actor, pid, application.SystemVoices()[0].ID, true); err != nil {
		t.Fatal(err)
	}
	system, err := s.ListSystemVoices(t.Context(), actor, pid)
	if err != nil || !system[0].Favorite {
		t.Fatal("system favorite readback", err)
	}
	if err = s.FavoriteSystemVoice(t.Context(), actor, pid, "invented-speaker", true); !errors.Is(err, application.ErrInvalidQuery) {
		t.Fatal("unverified speaker accepted", err)
	}
	if _, err = s.UpdateVoice(t.Context(), actor, pid, voice.ID, application.VoiceUpdate{Revision: updated.Revision, Name: updated.Name, Delete: true}); err != nil {
		t.Fatal(err)
	}
	page, err = s.ListVoices(t.Context(), actor, pid, uuid.Nil)
	if err != nil || len(page.Items) != 0 {
		t.Fatal("delete", err)
	}
	var count int64
	if err = db.Raw(`SELECT count(*) FROM media.media_asset WHERE id=? AND NOT is_delete`, id).Scan(&count).Error; err != nil || count != 1 {
		t.Fatal("source removed", err)
	}
	// Two tabs recovering the same unconfirmed request must converge on one record.
	concurrent := application.VoiceCreate{RequestID: uuid.New(), Name: "并发恢复", AssetID: id, Consent: true}
	type result struct {
		voice application.VoiceProfile
		err   error
	}
	results := make(chan result, 8)
	for range 8 {
		go func() { v, e := s.CreateVoice(t.Context(), actor, pid, concurrent); results <- result{v, e} }()
	}
	var first uuid.UUID
	var concurrentErrors []error
	for range 8 {
		r := <-results
		if r.err != nil {
			concurrentErrors = append(concurrentErrors, r.err)
			continue
		}
		if first == uuid.Nil {
			first = r.voice.ID
		}
		if r.voice.ID != first {
			concurrentErrors = append(concurrentErrors, errors.New("duplicate request created another voice"))
		}
	}
	if len(concurrentErrors) > 0 {
		t.Fatal(errors.Join(concurrentErrors...))
	}
	var created int64
	if err = db.Raw(`SELECT count(*) FROM media.voice_profile WHERE project_id=? AND actor_id=? AND request_id=?`, pid, actor.ID, concurrent.RequestID).Scan(&created).Error; err != nil || created != 1 {
		t.Fatal("concurrent receipt count", created, err)
	}

}
