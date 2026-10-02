package bible_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
)

func characterContent() domain.CharacterContent {
	return domain.CharacterContent{Name: "王总", Aliases: []string{"小王"}, Definition: domain.CharacterDefinition{Role: "主角", Appearance: "黑发"}, Looks: []domain.LookContent{{ID: uuid.New(), Name: "默认造型", Default: true}}}
}

func TestBibleCharacterClosedContentAndUnicodeBudgets(t *testing.T) {
	valid := characterContent()
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*domain.CharacterContent){
		func(c *domain.CharacterContent) { c.Name = strings.Repeat("角", 513) },
		func(c *domain.CharacterContent) { c.Looks = nil },
		func(c *domain.CharacterContent) { c.Looks[0].Default = false },
		func(c *domain.CharacterContent) { c.Looks = append(c.Looks, c.Looks[0]) },
		func(c *domain.CharacterContent) { c.Aliases = []string{"相同", "相同"} },
		func(c *domain.CharacterContent) { c.Definition.Appearance = string([]byte{0xff}) },
	} {
		c := characterContent()
		mutate(&c)
		if !errors.Is(c.Validate(), domain.ErrInvalidContent) {
			t.Fatal("accepted invalid content", c)
		}
	}
	valid.Name = strings.Repeat("𠮷", 512)
	if err := valid.Validate(); err != nil {
		t.Fatal("budget used UTF8 bytes", err)
	}
}

func TestBibleReferenceRolesAndFrozenMediaEvidence(t *testing.T) {
	c := characterContent()
	c.Looks[0].References = []domain.ImageReference{{Role: domain.RoleFront, Media: domain.MediaFact{AssetID: uuid.New(), Revision: 1, SHA256: strings.Repeat("a", 64), ByteSize: 42, Kind: "image", RenditionID: uuid.New(), RenditionSHA256: strings.Repeat("b", 64)}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Looks[0].References = append(c.Looks[0].References, c.Looks[0].References[0])
	if err := c.Validate(); !errors.Is(err, domain.ErrInvalidContent) {
		t.Fatal("duplicate role", err)
	}
	c.Looks[0].References = c.Looks[0].References[:1]
	c.Looks[0].References[0].Media.RenditionID = uuid.Nil
	if err := c.Validate(); !errors.Is(err, domain.ErrInvalidContent) {
		t.Fatal("unproved rendition", err)
	}
}

func TestBibleVoiceSourcesAreExclusiveAndSampleIsNotCatalog(t *testing.T) {
	c := characterContent()
	c.Voice = &domain.VoiceContent{Kind: domain.VoiceSample, Instructions: "轻声", Sample: &domain.SampleVoice{Name: "自己的样本", Media: domain.MediaFact{AssetID: uuid.New(), Revision: 3, SHA256: strings.Repeat("c", 64), ByteSize: 40, Kind: "audio"}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Voice.Catalog = &domain.CatalogVoice{ModelKey: "sample:fake", ModelVersionID: uuid.New(), ModelVersion: 1, VoiceKey: "alloy", ParamSchemaSHA256: strings.Repeat("d", 64)}
	if err := c.Validate(); !errors.Is(err, domain.ErrInvalidContent) {
		t.Fatal("mixed source", err)
	}
	c.Voice = &domain.VoiceContent{Kind: domain.VoiceCatalog, Catalog: &domain.CatalogVoice{ModelKey: "tts.configured", ModelVersionID: uuid.New(), ModelVersion: 1, VoiceKey: "alloy", ParamSchemaSHA256: strings.Repeat("d", 64)}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBibleCanonicalContentPreservesTextAndRejectsUnknownFields(t *testing.T) {
	c := characterContent()
	c.Description = "  e\u0301\n正文  "
	encoded, sha, err := domain.EncodeCharacter(c)
	if err != nil || len(sha) != 64 {
		t.Fatal(err)
	}
	decoded, err := domain.DecodeCharacter(encoded)
	if err != nil || decoded.Description != c.Description {
		t.Fatal("changed original text", decoded, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	raw["provider_json"] = json.RawMessage(`{"anything":true}`)
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.DecodeCharacter(b); !errors.Is(err, domain.ErrInvalidContent) {
		t.Fatal("accepted hidden fields", err)
	}
}
