package gormdb_test

import (
	"sync"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"gorm.io/gorm/schema"
)

func TestReferenceProviderCallBelongsToFrozenJob(t *testing.T) {
	parsed, err := schema.Parse(&model.GenerationReferenceProviderCall{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	job := parsed.Relationships.Relations["Job"]
	if job == nil || job.Type != schema.BelongsTo {
		t.Fatalf("call must reference Job, not own it: %+v", job)
	}
	constraint := job.ParseConstraint()
	if constraint == nil || constraint.Schema.Table != "gen_reference_provider_calls" || constraint.ReferenceSchema.Table != "gen_reference_provider_jobs" {
		t.Fatal("Provider call foreign key has the wrong owner")
	}
}

func TestReferenceStagedMediaBelongsToCall(t *testing.T) {
	parsed, err := schema.Parse(&model.GenerationReferenceStagedMedia{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatal(err)
	}
	call := parsed.Relationships.Relations["Call"]
	if call == nil || call.Type != schema.BelongsTo {
		t.Fatalf("staged media must reference Call, not own it: %+v", call)
	}
	constraint := call.ParseConstraint()
	if constraint == nil || constraint.Schema.Table != "gen_reference_staged_media" || constraint.ReferenceSchema.Table != "gen_reference_provider_calls" {
		t.Fatal("staged media foreign key has wrong owner")
	}
}
