package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/paramvalidation"
	pgcatalog "github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/postgres"
	catalogapp "github.com/StephenQiu30/lanverse/backend/internal/catalog/application"
)

func insertVoiceCatalogCapability(ctx context.Context, t *testing.T, database *gorm.DB) {
	t.Helper()
	if err := database.WithContext(ctx).Exec(`INSERT INTO catalog.capability (id,key,output_type,modes,input_roles) VALUES (?::uuid,'audio.tts','audio',ARRAY['tts'],ARRAY[]::text[]) ON CONFLICT(key) DO NOTHING`, uuid.NewString()).Error; err != nil {
		t.Fatal(err)
	}
}

func insertVoiceCatalogModel(ctx context.Context, t *testing.T, database *gorm.DB, key string, provider uuid.UUID, status string) (uuid.UUID, uuid.UUID) {
	t.Helper()
	model, version := insertModelCatalogModel(ctx, t, database, key, provider, "audio.tts", status, "tts", false)
	// This is an isolated owner fixture. Production configuration remains append-only.
	if err := database.WithContext(ctx).Exec(`UPDATE catalog.model_profile_version SET limits='{"max_outputs":1}'::jsonb, param_schema=?::jsonb WHERE id=?::uuid`, voiceCatalogSchema, version.String()).Error; err != nil {
		t.Fatal(err)
	}
	return model, version
}

func TestVoiceCatalogPGCurrentConfigurationScopeAndNonOwnerLocks(t *testing.T) {
	ctx, database := modelCatalogDB(t)
	org := insertModelCatalogOrganization(ctx, t, database)
	actor := insertModelCatalogActor(ctx, t, database, org)
	project := insertModelCatalogProject(ctx, t, database, org, false)
	foreignProject := insertModelCatalogProject(ctx, t, database, insertModelCatalogOrganization(ctx, t, database), false)
	insertVoiceCatalogCapability(ctx, t, database)
	provider := insertModelCatalogProvider(ctx, t, database, "domestic", "active", "ok")
	prefix := "voice-catalog-" + uuid.NewString()
	_, version := insertVoiceCatalogModel(ctx, t, database, prefix+"-a", provider, "active")
	insertVoiceCatalogModel(ctx, t, database, prefix+"-b", provider, "disabled")
	insertVoiceCatalogModel(ctx, t, database, prefix+"-c", insertModelCatalogProvider(ctx, t, database, "overseas", "active", "ok"), "active")
	insertVoiceCatalogModel(ctx, t, database, prefix+"-d", insertModelCatalogProvider(ctx, t, database, "domestic", "active", "auth_failed"), "active")
	validator, err := paramvalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
			return err
		}
		var role string
		if err := tx.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "lanverse_app" {
			t.Fatalf("missing actual non-owner role: %s %v", role, err)
		}
		q := catalogapp.NewVoiceCatalog(pgcatalog.NewStore(tx), validator)
		fact, err := q.Reference(ctx, actor, project, catalogapp.VoiceSelection{ModelKey: prefix + "-a", ExpectedVersion: 1, VoiceKey: "a_voice", Params: json.RawMessage(`{"speed":1.2}`)})
		if err != nil || fact.ModelVersionID != version || fact.ModelKey != prefix+"-a" {
			t.Fatalf("actual published binding failed: %+v %v", fact, err)
		}
		for _, key := range []string{prefix + "-b", prefix + "-c", prefix + "-d"} {
			_, err := q.Reference(ctx, actor, project, catalogapp.VoiceSelection{ModelKey: key, ExpectedVersion: 1, VoiceKey: "a_voice", Params: json.RawMessage(`{}`)})
			if !errors.Is(err, catalogapp.ErrVoiceModelNotFound) {
				t.Fatalf("unavailable model %s became bindable: %v", key, err)
			}
		}
		_, err = q.List(ctx, actor, foreignProject, 10, nil)
		if !errors.Is(err, catalogapp.ErrModelCatalogProjectNotFound) {
			t.Fatalf("foreign project catalog disclosed: %v", err)
		}
		page, err := q.List(ctx, actor, project, 1, &catalogapp.VoiceCursor{ModelKey: prefix + "-a", VoiceKey: "a_voice"})
		if err != nil || len(page.Voices) != 1 || page.Voices[0].ModelKey != prefix+"-a" || page.Voices[0].VoiceKey != "z_voice" {
			t.Fatalf("actual voice pagination failed: %+v %v", page, err)
		}
		// A second owner transaction must not revoke the fact during the caller write.
		for name, statement := range map[string]string{
			"model":      `UPDATE catalog.model_profile SET status='disabled' WHERE model_key=?`,
			"provider":   `UPDATE catalog.provider SET status='disabled' WHERE id=?::uuid`,
			"credential": `UPDATE catalog.provider_credential SET status='disabled' WHERE provider_id=?::uuid AND status='active'`,
		} {
			argument := provider.String()
			if name == "model" {
				argument = prefix + "-a"
			}
			err := database.WithContext(ctx).Transaction(func(other *gorm.DB) error {
				if err := other.Exec(`SET LOCAL lock_timeout='100ms'`).Error; err != nil {
					return err
				}
				return other.Exec(statement, argument).Error
			})
			var locked *pgconn.PgError
			if !errors.As(err, &locked) || locked.Code != "55P03" {
				t.Fatalf("%s configuration lock did not survive caller transaction: %v", name, err)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
