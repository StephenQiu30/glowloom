package application

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

// ErrVoiceConflict prevents stale updates and conflicting request replays.
var ErrVoiceConflict = errors.New("voice revision conflict")

// VoiceProfile is an actor-private reference voice, not a trained supplier voice.
type VoiceProfile struct {
	VoiceTags
	ID            uuid.UUID `json:"id"`
	ProjectID     uuid.UUID `json:"project_id"`
	Name          string    `json:"name"`
	AssetID       uuid.UUID `json:"asset_id"`
	AssetRevision int64     `json:"asset_revision"`
	Revision      int64     `json:"revision"`
	Favorite      bool      `json:"favorite"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// VoiceCreate identifies a replayable consented reference selection.
type VoiceCreate struct {
	VoiceTags
	RequestID uuid.UUID `json:"request_id"`
	Name      string    `json:"name"`
	AssetID   uuid.UUID `json:"asset_id"`
	Consent   bool      `json:"consent"`
}

// VoiceUpdate changes metadata under optimistic concurrency.
type VoiceUpdate struct {
	Revision int64  `json:"revision"`
	Name     string `json:"name"`
	Favorite bool   `json:"favorite"`
	Delete   bool   `json:"delete"`
}

// VoicePage is a bounded owner-scoped collection.
type VoicePage struct {
	Items      []VoiceProfile `json:"items"`
	NextCursor *uuid.UUID     `json:"next_cursor"`
}

// VoiceLibrary keeps authorization, source validation and writes in one transaction.
type VoiceLibrary interface {
	ListSystemVoices(context.Context, identity.Principal, uuid.UUID) ([]SystemVoice, error)
	FavoriteSystemVoice(context.Context, identity.Principal, uuid.UUID, string, bool) error
	CreateVoice(context.Context, identity.Principal, uuid.UUID, VoiceCreate) (VoiceProfile, error)
	ListVoices(context.Context, identity.Principal, uuid.UUID, uuid.UUID) (VoicePage, error)
	UpdateVoice(context.Context, identity.Principal, uuid.UUID, uuid.UUID, VoiceUpdate) (VoiceProfile, error)
}

// ValidateReferenceVoice rejects unconsented or unsuitable audio before persistence.
func ValidateReferenceVoice(name string, consent bool, asset AssetSummary) error {
	if !ValidVoiceName(name) || !consent || asset.ID == uuid.Nil || asset.Kind != "audio" || !strings.HasPrefix(asset.MIMEType, "audio/") || asset.Revision < 1 || asset.ByteSize < 1 || asset.ByteSize > 10*1024*1024 || asset.DurationMS == nil || *asset.DurationMS <= 0 || *asset.DurationMS > 30000 {
		return ErrInvalidQuery
	}
	return nil
}

// ValidVoiceName bounds display metadata, including non-ASCII names.
func ValidVoiceName(name string) bool {
	return strings.TrimSpace(name) == name && name != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= 60
}

// SystemVoice is a documented Seed TTS 2.0 speaker. No sample URL is invented.
type SystemVoice struct {
	VoiceTags
	ID       string `json:"id"`
	Name     string `json:"name"`
	Favorite bool   `json:"favorite"`
}

// SystemVoices returns the verified official subset, independently of releases.
func SystemVoices() []SystemVoice {
	return []SystemVoice{
		{VoiceTags: VoiceTags{Gender: "女声", Age: "未标注", Language: "中文", Style: "未标注"}, ID: "zh_female_vv_uranus_bigtts", Name: "Vivi 2.0"},
		{VoiceTags: VoiceTags{Gender: "男声", Age: "未标注", Language: "中文", Style: "未标注"}, ID: "zh_male_yangguangqingnian_uranus_bigtts", Name: "阳光青年 2.0"},
		{VoiceTags: VoiceTags{Gender: "男声", Age: "未标注", Language: "中文", Style: "未标注"}, ID: "zh_male_wenrouxiaoge_uranus_bigtts", Name: "温柔小哥 2.0"},
		{VoiceTags: VoiceTags{Gender: "女声", Age: "未标注", Language: "中文", Style: "未标注"}, ID: "zh_female_wenrouxiaoya_uranus_bigtts", Name: "温柔小雅 2.0"},
		{VoiceTags: VoiceTags{Gender: "女声", Age: "未标注", Language: "中文", Style: "未标注"}, ID: "zh_female_qinqienv_uranus_bigtts", Name: "亲切女声 2.0"},
		{VoiceTags: VoiceTags{Gender: "女声", Age: "未标注", Language: "中文", Style: "未标注"}, ID: "zh_female_zhixingnv_uranus_bigtts", Name: "知性女声 2.0"},
		{VoiceTags: VoiceTags{Gender: "男声", Age: "未标注", Language: "中文", Style: "未标注"}, ID: "zh_male_guanggaojieshuo_uranus_bigtts", Name: "广告解说 2.0"},
	}
}

// VoiceTags are user-provided reference metadata, never inferred demographics.
type VoiceTags struct {
	Gender   string `json:"gender"`
	Age      string `json:"age"`
	Language string `json:"language"`
	Style    string `json:"style"`
}

// Normalize fills unspecified categories and rejects unsupported filter values.
func (t *VoiceTags) Normalize() error {
	choices := []struct {
		value   *string
		allowed []string
	}{
		{&t.Gender, []string{"未标注", "男声", "女声"}},
		{&t.Age, []string{"未标注", "儿童", "青年", "中年", "老年"}},
		{&t.Language, []string{"未标注", "中文", "英文", "日文", "其他"}},
		{&t.Style, []string{"未标注", "自然", "温柔", "活泼", "沉稳", "旁白"}},
	}
	for _, choice := range choices {
		if *choice.value == "" {
			*choice.value = "未标注"
		}
		valid := false
		for _, v := range choice.allowed {
			if *choice.value == v {
				valid = true
			}
		}
		if !valid {
			return ErrInvalidQuery
		}
	}
	return nil
}
