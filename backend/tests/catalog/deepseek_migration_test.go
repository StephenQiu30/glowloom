package catalog_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/db"
)

func TestDeepSeekPresetIncremental(t *testing.T) {
	dsn := os.Getenv("GLOWLOOM_TEST_CATALOG_DB_DSN")
	if dsn == "" {
		t.Skip("set GLOWLOOM_TEST_CATALOG_DB_DSN to an isolated migrated database")
	}
	conn, err := db.Open(t.Context(), dsn, noop.NewTracerProvider())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	migration, err := os.ReadFile("../../db/deepseek_provider_incremental.sql")
	require.NoError(t, err)

	for _, state := range []string{"missing", "existing", "custom"} {
		t.Run(state, func(t *testing.T) {
			tx := conn.DB.WithContext(t.Context()).Begin()
			require.NoError(t, tx.Error)
			t.Cleanup(func() { _ = tx.Rollback().Error })
			// Clone only the registry tables into a transaction-owned schema. The
			// real isolated database's presets, versions and references stay intact.
			schema := "preset_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
			require.NoError(t, tx.Exec("CREATE SCHEMA "+schema).Error)
			require.NoError(t, tx.Exec("CREATE TABLE "+schema+".provider_preset (LIKE catalog.provider_preset INCLUDING ALL)").Error)
			require.NoError(t, tx.Exec("CREATE TABLE "+schema+".provider_preset_version (LIKE catalog.provider_preset_version INCLUDING ALL)").Error)
			require.NoError(t, tx.Exec("ALTER TABLE "+schema+".provider_preset_version ADD FOREIGN KEY(preset_id) REFERENCES "+schema+".provider_preset(id)").Error)
			seedID := "6da07a10-0000-4000-8000-000000000004"
			if state != "missing" {
				origin := "builtin"
				if state == "custom" {
					origin = "custom"
					seedID = uuid.NewString()
				}
				require.NoError(t, tx.Exec("INSERT INTO "+schema+".provider_preset(id,slug,origin,archived,emergency_disabled) VALUES(?,'deepseek',?,true,true)", seedID, origin).Error)
				if state == "existing" {
					require.NoError(t, tx.Exec("INSERT INTO "+schema+".provider_preset_version(id,preset_id,version_no,payload,content_hash) VALUES(?,?,1,'{\"historical\":true}'::jsonb,?)", uuid.New(), seedID, strings.Repeat("a", 64)).Error)
				}
			}
			var before string
			if state != "missing" {
				require.NoError(t, tx.Raw("SELECT to_jsonb(p)::text FROM "+schema+".provider_preset p").Scan(&before).Error)
			}
			sql := strings.ReplaceAll(string(migration), "catalog.", schema+".")
			require.NoError(t, tx.Exec(sql).Error)
			var first string
			require.NoError(t, tx.Raw("SELECT jsonb_build_object('preset',(SELECT jsonb_agg(to_jsonb(p)) FROM "+schema+".provider_preset p),'versions',(SELECT jsonb_agg(to_jsonb(v)) FROM "+schema+".provider_preset_version v))::text").Scan(&first).Error)
			require.NoError(t, tx.Exec(sql).Error)
			var second string
			require.NoError(t, tx.Raw("SELECT jsonb_build_object('preset',(SELECT jsonb_agg(to_jsonb(p)) FROM "+schema+".provider_preset p),'versions',(SELECT jsonb_agg(to_jsonb(v)) FROM "+schema+".provider_preset_version v))::text").Scan(&second).Error)
			require.Equal(t, first, second, "re-running the migration must preserve every registry field")
			if state == "missing" {
				var row struct {
					Payload     []byte
					ContentHash string
				}
				require.NoError(t, tx.Raw("SELECT payload,content_hash FROM "+schema+".provider_preset_version").Scan(&row).Error)
				var document domain.PresetDocument
				require.NoError(t, json.Unmarshal(row.Payload, &document))
				require.NoError(t, document.Validate())
				require.Equal(t, "deepseek", document.Config.AdapterKey)
				require.Equal(t, document.Digest(), row.ContentHash)
			} else {
				var after string
				require.NoError(t, tx.Raw("SELECT to_jsonb(p)::text FROM "+schema+".provider_preset p").Scan(&after).Error)
				require.Equal(t, before, after, "existing identity and disabled flags must stay intact")
				var count int64
				require.NoError(t, tx.Table(schema+".provider_preset_version").Count(&count).Error)
				if state == "custom" {
					require.Zero(t, count, "a custom slug must never receive a built-in version")
				} else {
					require.EqualValues(t, 1, count)
					require.Contains(t, first, `"historical": true`)
				}
			}
		})
	}
}
