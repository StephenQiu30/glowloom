package postgres

import (
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

type voiceProfileRecord struct {
	ID            uuid.UUID
	ProjectID     uuid.UUID
	ActorID       uuid.UUID
	RequestID     uuid.UUID
	Name          string
	RequestName   string
	AssetID       uuid.UUID
	AssetRevision int64
	ConsentAt     time.Time
	Revision      int64
	Favorite      bool
	IsDelete      bool
	Gender        string
	Age           string
	Language      string
	Style         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (voiceProfileRecord) TableName() string { return "media.voice_profile" }

func (r voiceProfileRecord) profile() application.VoiceProfile {
	return application.VoiceProfile{
		VoiceTags: application.VoiceTags{Gender: r.Gender, Age: r.Age, Language: r.Language, Style: r.Style},
		ID:        r.ID, ProjectID: r.ProjectID, Name: r.Name, AssetID: r.AssetID,
		AssetRevision: r.AssetRevision, Revision: r.Revision, Favorite: r.Favorite, UpdatedAt: r.UpdatedAt,
	}
}

type voiceFavoriteRecord struct {
	ProjectID uuid.UUID
	ActorID   uuid.UUID
	Speaker   string
	Favorite  bool
}

func (voiceFavoriteRecord) TableName() string { return "media.voice_favorite" }
