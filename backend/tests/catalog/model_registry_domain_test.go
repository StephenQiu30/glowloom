package catalog_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/domain"
)

func validModelProfile() domain.ModelProfile {
	return domain.ModelProfile{
		ID: uuid.New(), Key: "seedance-2", ProviderID: uuid.New(),
		Capability: "video.generate", DisplayName: "Seedance 2",
		Status: domain.ModelDisabled, Revision: 1,
	}
}

func validModelVersion(modelID uuid.UUID) domain.ModelVersion {
	return domain.ModelVersion{
		ID: uuid.New(), ModelID: modelID, VersionNo: 1,
		ProviderModelID: "seedance-2", Modes: []string{"image2video"},
		Limits: []byte(`{"max_outputs":1}`), ParamSchema: []byte(`[]`),
		ExpectedMaxMS: 60000, Moderation: domain.ModerationProvider, Queue: "agent",
	}
}

func validPriceVersion(modelID uuid.UUID) domain.PriceRuleVersion {
	return domain.PriceRuleVersion{
		ID: uuid.New(), ModelID: modelID, VersionNo: 1,
		Unit: domain.PricePerSecond, Rule: []byte(`{"base_micros":1000}`),
		Currency: "CNY", EffectiveFrom: time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC),
	}
}

func TestModelRegistryDomainRejectsInvalidPayloads(t *testing.T) {
	capability := domain.Capability{
		ID: uuid.New(), Key: "video.generate", OutputType: domain.OutputVideo,
		Modes: []string{"image2video"}, InputRoles: []string{"subject"},
	}
	if err := capability.Validate(); err != nil {
		t.Fatal(err)
	}
	capability.Modes = []string{"image2video", "image2video"}
	if err := capability.Validate(); !errors.Is(err, domain.ErrInvalidCapability) {
		t.Fatalf("duplicate capability mode accepted: %v", err)
	}
	model := validModelProfile()
	if err := model.Validate(); err != nil {
		t.Fatal(err)
	}
	model.Revision = 0
	if err := model.Validate(); !errors.Is(err, domain.ErrInvalidModel) {
		t.Fatalf("zero revision accepted: %v", err)
	}
	version := validModelVersion(model.ID)
	if err := version.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*domain.ModelVersion){
		func(v *domain.ModelVersion) { v.VersionNo = 0 },
		func(v *domain.ModelVersion) { v.Limits = []byte(`[]`) },
		func(v *domain.ModelVersion) { v.ParamSchema = []byte(`{}`) },
		func(v *domain.ModelVersion) { v.Modes = nil },
		func(v *domain.ModelVersion) { v.ExpectedMaxMS = 0 },
	} {
		candidate := version
		mutate(&candidate)
		if err := candidate.Validate(); !errors.Is(err, domain.ErrInvalidModelVersion) {
			t.Fatalf("invalid model version accepted: %v", err)
		}
	}
	price := validPriceVersion(model.ID)
	if err := price.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*domain.PriceRuleVersion){
		func(p *domain.PriceRuleVersion) { p.Rule = []byte(`[]`) },
		func(p *domain.PriceRuleVersion) { p.Currency = "usd" },
		func(p *domain.PriceRuleVersion) { p.Currency = "USD" },
		func(p *domain.PriceRuleVersion) { p.Currency = "USD"; p.FXRateToCNY = "0" },
		func(p *domain.PriceRuleVersion) { p.Currency = "USD"; p.FXRateToCNY = "1.1234567" },
		func(p *domain.PriceRuleVersion) { p.Currency = "USD"; p.FXRateToCNY = "1234567" },
		func(p *domain.PriceRuleVersion) { p.EffectiveFrom = time.Time{} },
	} {
		candidate := price
		mutate(&candidate)
		if err := candidate.Validate(); !errors.Is(err, domain.ErrInvalidPriceRule) {
			t.Fatalf("invalid price version accepted: %v", err)
		}
	}
	price.Currency, price.FXRateToCNY = "USD", "7.123456"
	if err := price.Validate(); err != nil {
		t.Fatalf("valid versioned exchange rate rejected: %v", err)
	}
}

func TestModelStatusAndVersionTransitions(t *testing.T) {
	model := validModelProfile()
	if err := model.SetStatus(domain.ModelActive, true); !errors.Is(err, domain.ErrModelNotPublishable) {
		t.Fatalf("model without version activated: %v", err)
	}
	version := validModelVersion(model.ID)
	if err := model.AttachVersion(version, 2); !errors.Is(err, domain.ErrModelRevisionConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	foreign := validModelVersion(uuid.New())
	if err := model.AttachVersion(foreign, 1); !errors.Is(err, domain.ErrInvalidModelVersion) {
		t.Fatalf("foreign model version attached: %v", err)
	}
	if err := model.AttachVersion(version, 1); err != nil || model.CurrentVersionID != version.ID || model.Revision != 2 {
		t.Fatalf("attach model version: %+v, %v", model, err)
	}
	if err := model.SetStatus(domain.ModelActive, false); !errors.Is(err, domain.ErrModelNotPublishable) {
		t.Fatalf("model without price activated: %v", err)
	}
	if err := model.SetStatus(domain.ModelActive, true); err != nil || model.Status != domain.ModelActive || model.Revision != 3 {
		t.Fatalf("activate model: %+v, %v", model, err)
	}
	if err := model.SetStatus(domain.ModelActive, true); !errors.Is(err, domain.ErrInvalidModelTransition) {
		t.Fatalf("repeated activation accepted: %v", err)
	}
	if err := model.SetStatus(domain.ModelDisabled, false); err != nil || model.Status != domain.ModelDisabled || model.Revision != 4 {
		t.Fatalf("disable model: %+v, %v", model, err)
	}
}
