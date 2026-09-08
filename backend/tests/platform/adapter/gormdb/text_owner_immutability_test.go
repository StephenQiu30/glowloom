package gormdb_test

import (
	"strings"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/database/model"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAcceptedTextOwnersRejectGORMUpdatesAndDeletes(t *testing.T) {
	db, err := gorm.Open(postgres.Open("host=127.0.0.1 user=unused dbname=unused sslmode=disable"), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	for name, value := range map[string]any{
		"world":  &model.TextWorldVersion{ID: uuid.New()},
		"intent": &model.TextIntentVersion{ID: uuid.New()},
	} {
		t.Run(name, func(t *testing.T) {
			if err := db.Model(value).Update("content_hash", strings.Repeat("f", 64)).Error; err == nil || !strings.Contains(err.Error(), "immutable") {
				t.Fatalf("accepted owner update was allowed: %v", err)
			}
			if err := db.Delete(value).Error; err == nil || !strings.Contains(err.Error(), "immutable") {
				t.Fatalf("accepted owner deletion was allowed: %v", err)
			}
		})
	}
}
