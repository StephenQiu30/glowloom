package config_test

import (
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/config"
)

func TestLoadBuildsSingleDatabaseAPIConfiguration(t *testing.T) {
	t.Setenv("API_HOST", "127.0.0.1")
	t.Setenv("API_PORT", "8765")
	t.Setenv("DATABASE_URL", "postgresql://lanverse:secret@database:5432/lanverse")

	configuration, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.ListenAddress != "127.0.0.1:8765" {
		t.Fatalf("listen address = %q", configuration.ListenAddress)
	}
	if configuration.DatabaseURL != "postgresql://lanverse:secret@database:5432/lanverse" {
		t.Fatalf("database URL = %q", configuration.DatabaseURL)
	}
	if configuration.AccessTokenTTL != 30*time.Minute || configuration.SessionTTL != 30*24*time.Hour {
		t.Fatalf("unexpected token TTLs: %s, %s", configuration.AccessTokenTTL, configuration.SessionTTL)
	}
	if len(configuration.AllowedOrigins) != 2 {
		t.Fatalf("allowed origins = %#v", configuration.AllowedOrigins)
	}
	if configuration.ObjectStoreRegion != "us-east-1" {
		t.Fatalf("object store region = %q", configuration.ObjectStoreRegion)
	}
	if configuration.AgentURL != "http://127.0.0.1:8787" || configuration.AgentPollInterval != 500*time.Millisecond {
		t.Fatalf("unexpected agent configuration: %q, %s", configuration.AgentURL, configuration.AgentPollInterval)
	}
	if configuration.TemporalAddress != "127.0.0.1:7233" || configuration.TemporalNamespace != "default" ||
		configuration.TemporalTaskQueue != "lanverse-production-v1" {
		t.Fatalf("unexpected Temporal configuration: %#v", configuration)
	}
	if configuration.EventWorkerListenAddress != "0.0.0.0:8687" || len(configuration.KafkaBrokers) != 1 ||
		configuration.KafkaBrokers[0] != "127.0.0.1:9092" ||
		configuration.KafkaScriptTopic == configuration.KafkaScriptDLQTopic ||
		configuration.KafkaStoryGraphTopic == configuration.KafkaStoryGraphDLQTopic ||
		configuration.ElasticsearchURL != "http://127.0.0.1:9200" ||
		configuration.ElasticsearchScriptAlias == configuration.ElasticsearchStoryGraphAlias {
		t.Fatalf("unexpected Kafka eventing configuration: %#v", configuration)
	}
}

func TestLoadRejectsInvalidTemporalAddress(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://lanverse:secret@database:5432/lanverse")
	for _, value := range []string{"temporal", "http://temporal:7233", "temporal:99999"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("TEMPORAL_ADDRESS", value)
			if _, err := config.Load(); err == nil {
				t.Fatalf("Load accepted TEMPORAL_ADDRESS %q", value)
			}
		})
	}
}

func TestLoadRejectsInvalidCORSOrigins(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://lanverse:secret@database:5432/lanverse")
	t.Setenv("CORS_ORIGINS", `["not-an-origin"]`)
	if _, err := config.Load(); err == nil {
		t.Fatal("Load accepted an invalid CORS origin")
	}
}

func TestLoadRejectsInvalidConfiguredRegistrationCode(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://lanverse:secret@database:5432/lanverse")
	t.Setenv("REGISTRATION_VERIFICATION_CODE", "12345")
	if _, err := config.Load(); err == nil {
		t.Fatal("Load accepted an invalid registration verification code")
	}
}

func TestLoadRequiresStandardPostgreSQLDatabaseURL(t *testing.T) {
	for _, value := range []string{"", "postgresql+asyncpg://database/lanverse", "http://database/lanverse"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", value)
			if _, err := config.Load(); err == nil {
				t.Fatalf("Load accepted DATABASE_URL %q", value)
			}
		})
	}
}

func TestLoadRejectsInvalidOrSharedKafkaDestinations(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://lanverse:secret@database:5432/lanverse")
	for name, values := range map[string]map[string]string{
		"invalid broker": {"KAFKA_BROKERS": "http://kafka:9092"},
		"shared dlq": {
			"KAFKA_STORYGRAPH_TOPIC":     "lanverse.business.storygraph.v1",
			"KAFKA_STORYGRAPH_DLQ_TOPIC": "lanverse.business.storygraph.v1",
		},
		"shared business topics": {
			"KAFKA_SCRIPT_TOPIC":     "lanverse.business.shared.v1",
			"KAFKA_STORYGRAPH_TOPIC": "lanverse.business.shared.v1",
		},
		"invalid topic": {"KAFKA_STORYGRAPH_TOPIC": "storygraph command topic"},
	} {
		t.Run(name, func(t *testing.T) {
			for key, value := range values {
				t.Setenv(key, value)
			}
			if _, err := config.Load(); err == nil {
				t.Fatal("Load accepted invalid Kafka configuration")
			}
		})
	}
}

func TestLoadRejectsInvalidOrSharedElasticsearchDestinations(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://lanverse:secret@database:5432/lanverse")
	for name, values := range map[string]map[string]string{
		"invalid URL": {"ELASTICSEARCH_URL": "elasticsearch:9200"},
		"URL path":    {"ELASTICSEARCH_URL": "http://elasticsearch:9200/index"},
		"shared alias": {
			"ELASTICSEARCH_SCRIPT_ALIAS":     "lanverse-search-v1",
			"ELASTICSEARCH_STORYGRAPH_ALIAS": "lanverse-search-v1",
		},
	} {
		t.Run(name, func(t *testing.T) {
			for key, value := range values {
				t.Setenv(key, value)
			}
			if _, err := config.Load(); err == nil {
				t.Fatal("Load accepted invalid Elasticsearch configuration")
			}
		})
	}
}
