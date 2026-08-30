package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackendHasOneRuntimeEntrypoint(t *testing.T) {
	t.Parallel()

	repositoryRoot := repositoryDirectory(t)
	commandRoot := filepath.Join(repositoryRoot, "backend", "cmd")
	entries, err := os.ReadDir(commandRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].IsDir() || entries[0].Name() != "main.go" {
		t.Fatalf("backend/cmd must contain only main.go, got %v", entryNames(entries))
	}

	mainSource := readArchitectureFile(t, filepath.Join(commandRoot, "main.go"))
	for _, runtime := range []string{
		"bootstrap.RunAPI(runtimeContext, logger)",
		"bootstrap.RunWorkflowWorker(ctx, logger)",
		"bootstrap.RunEventWorker(ctx, logger)",
	} {
		if !strings.Contains(mainSource, runtime) {
			t.Errorf("single Backend entrypoint does not start %s", runtime)
		}
	}
	if !strings.Contains(mainSource, "signal.NotifyContext(") || !strings.Contains(mainSource, "superviseRuntime(") {
		t.Error("single Backend entrypoint must own shutdown signals and runtime retries")
	}
	for _, component := range []string{"api_process.go", "workflow_process.go", "event_process.go"} {
		source := readArchitectureFile(t, filepath.Join(repositoryRoot, "backend", "internal", "bootstrap", component))
		if strings.Contains(source, "os.Exit(") {
			t.Errorf("%s can terminate the unified Backend process directly", component)
		}
		if strings.Contains(source, "signal.NotifyContext(") {
			t.Errorf("%s owns a process signal outside the single entrypoint", component)
		}
	}

	dockerfile := readArchitectureFile(t, filepath.Join(repositoryRoot, "backend", "Dockerfile"))
	for _, forbidden := range []string{"lanverse-api", "lanverse-workflow-worker", "lanverse-event-worker"} {
		if strings.Contains(dockerfile, forbidden) {
			t.Errorf("Backend image still contains obsolete binary %q", forbidden)
		}
	}
	for _, required := range []string{
		"ENTRYPOINT [\"/usr/local/bin/docker-entrypoint.sh\"]",
		"CMD [\"/usr/local/bin/lanverse\"]",
		"CMD su-exec lanverse:lanverse wget",
		"apk add --no-cache bash ca-certificates curl su-exec",
		"COPY observability/elasticsearch/init.sh /usr/share/lanverse-observability/elasticsearch-init.sh",
		"COPY observability/elasticsearch/ilm-policy.json /usr/share/lanverse-observability/ilm-policy.json",
		"COPY observability/logstash/template/lanverse-logs-template.json /usr/share/lanverse-observability/lanverse-logs-template.json",
		"COPY observability/kibana/init.sh /usr/share/lanverse-observability/kibana-init.sh",
		"su-exec",
	} {
		if !strings.Contains(dockerfile, required) {
			t.Errorf("Backend image is missing non-root single-binary startup contract %q", required)
		}
	}
	entrypoint := readArchitectureFile(t, filepath.Join(repositoryRoot, "backend", "docker-entrypoint.sh"))
	for _, required := range []string{
		"umask 077",
		"[ -f \"$mounted_root_key\" ] && [ -s \"$mounted_root_key\" ]",
		"/run/secrets/lanverse_media_provider_master_key_source",
		"/run/secrets/lanverse_media_provider_master_key",
		"stat -f -c %T /run/secrets",
		"install -o lanverse -g lanverse -m 0400",
		"/usr/share/lanverse-observability/elasticsearch-init.sh",
		"/usr/share/lanverse-observability/kibana-init.sh",
		"unset ELASTICSEARCH_INIT_USERNAME ELASTICSEARCH_INIT_PASSWORD KIBANA_USERNAME KIBANA_PASSWORD",
		"exec su-exec lanverse:lanverse",
	} {
		if !strings.Contains(entrypoint, required) {
			t.Errorf("Backend entrypoint is missing root-key handoff contract %q", required)
		}
	}
	ciWorkflow := readArchitectureFile(t, filepath.Join(repositoryRoot, ".github", "workflows", "ci.yml"))
	if !strings.Contains(ciWorkflow, `tr "\000" "\n" < /proc/1/cmdline`) {
		t.Error("deployment CI must verify the non-root Backend PID 1 command without ptrace")
	}
	if strings.Contains(ciWorkflow, "readlink /proc/1/exe") {
		t.Error("deployment CI cannot require ptrace access to a different-UID Backend PID 1")
	}
	for _, required := range []string{
		"ELASTIC_OBSERVABILITY_NETWORK: lanverse-environment",
		"DOCKER_ELASTICSEARCH_URL: http://elasticsearch:9200",
		"DOCKER_LOGSTASH_ADDRESS: logstash:5000",
		"DOCKER_KIBANA_URL: http://kibana:5601",
	} {
		if !strings.Contains(ciWorkflow, required) {
			t.Errorf("deployment CI does not reuse its existing ELK network via %q", required)
		}
	}

	apiSource := readArchitectureFile(t, filepath.Join(repositoryRoot, "backend", "internal", "bootstrap", "api_process.go"))
	for _, required := range []string{
		"generationapp.NewMediaFactoryRegistry(nil)",
		"generationapp.NewProviderConfigurationService(",
		"providersecret.OpenFixed()",
	} {
		if !strings.Contains(apiSource, required) {
			t.Errorf("single Backend API composition is missing %q", required)
		}
	}
	for _, forbidden := range []string{"Runware", "IMAGE_PROVIDER", "RUNWARE_API_KEY", "NewProviderBindingHandler"} {
		if strings.Contains(apiSource, forbidden) {
			t.Errorf("Backend API composition still contains obsolete Provider configuration %q", forbidden)
		}
	}

	playwrightConfig := readArchitectureFile(t, filepath.Join(repositoryRoot, "frontend", "playwright.config.ts"))
	if !strings.Contains(playwrightConfig, "go run ./cmd") || strings.Contains(playwrightConfig, "go run ./cmd/api") {
		t.Error("Playwright must start the single Backend entrypoint")
	}

	compose := readArchitectureFile(t, filepath.Join(repositoryRoot, "docker-compose.yml"))
	for _, required := range []string{
		"source: lanverse_media_provider_master_key",
		"target: lanverse_media_provider_master_key_source",
		"/run/secrets:rw,noexec,nosuid,nodev,mode=0711",
		"no-new-privileges:true",
		"test: [\"CMD\", \"su-exec\", \"lanverse:lanverse\", \"wget\"",
		"file: ${LANVERSE_MEDIA_PROVIDER_MASTER_KEY_FILE:-/dev/null}",
		"ELASTICSEARCH_INIT_USERNAME: ${ELASTICSEARCH_INIT_USERNAME:-}",
		"ELASTICSEARCH_INIT_PASSWORD: ${ELASTICSEARCH_INIT_PASSWORD:-}",
		"KIBANA_URL: ${DOCKER_KIBANA_URL:-http://kibana-local-dev:5601}",
		"KIBANA_USERNAME: ${KIBANA_USERNAME:-}",
		"KIBANA_PASSWORD: ${KIBANA_PASSWORD:-}",
		"- observability",
		"name: ${ELASTIC_OBSERVABILITY_NETWORK:-elastic-start-local_default}",
		"external: true",
	} {
		if !strings.Contains(compose, required) {
			t.Errorf("service Compose is missing Provider root-key contract %q", required)
		}
	}
	for _, forbidden := range []string{"IMAGE_PROVIDER", "RUNWARE_API_KEY", "RUNWARE_REQUEST_TIMEOUT_SECONDS"} {
		if strings.Contains(compose, forbidden) {
			t.Errorf("service Compose still exposes obsolete Provider environment variable %q", forbidden)
		}
	}
	for _, environmentTemplate := range []string{".env.example", ".env.production.example"} {
		source := readArchitectureFile(t, filepath.Join(repositoryRoot, environmentTemplate))
		if !strings.Contains(source, "LANVERSE_MEDIA_PROVIDER_MASTER_KEY_FILE=") {
			t.Errorf("%s is missing the Provider root-key file contract", environmentTemplate)
		}
		for _, forbidden := range []string{"IMAGE_PROVIDER=", "RUNWARE_API_KEY=", "RUNWARE_REQUEST_TIMEOUT_SECONDS="} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s still exposes obsolete Provider environment variable %q", environmentTemplate, forbidden)
			}
		}
		for _, required := range []string{
			"ELASTICSEARCH_INIT_USERNAME=",
			"ELASTICSEARCH_INIT_PASSWORD=",
			"KIBANA_URL=",
			"DOCKER_KIBANA_URL=",
			"KIBANA_USERNAME=",
			"KIBANA_PASSWORD=",
		} {
			if !strings.Contains(source, required) {
				t.Errorf("%s is missing the observability startup contract %q", environmentTemplate, required)
			}
		}
	}
	developmentEnvironment := readArchitectureFile(t, filepath.Join(repositoryRoot, ".env.example"))
	for _, required := range []string{
		"ELASTIC_OBSERVABILITY_NETWORK=elastic-start-local_default",
		"DOCKER_ELASTICSEARCH_URL=http://elasticsearch:9200",
		"DOCKER_LOGSTASH_ADDRESS=logstash-local-dev:5000",
		"DOCKER_KIBANA_URL=http://kibana-local-dev:5601",
	} {
		if !strings.Contains(developmentEnvironment, required) {
			t.Errorf("development environment template does not reuse the existing ELK network via %q", required)
		}
	}
	for _, forbidden := range []string{"\n  workflow-worker:", "\n  event-worker:"} {
		if strings.Contains(compose, forbidden) {
			t.Errorf("project Compose still declares obsolete service %q", strings.TrimSpace(forbidden))
		}
	}
	for _, environmentService := range []string{"postgres", "minio", "temporal", "kafka", "elasticsearch", "filebeat", "logstash", "kibana"} {
		if strings.Contains(compose, "\n  "+environmentService+":") {
			t.Errorf("service Compose owns environment service %q", environmentService)
		}
	}
	environmentCompose := readArchitectureFile(t, filepath.Join(repositoryRoot, "docker-compose-env.yml"))
	for _, applicationService := range []string{"backend", "frontend"} {
		if strings.Contains(environmentCompose, "\n  "+applicationService+":") {
			t.Errorf("environment Compose owns application service %q", applicationService)
		}
	}
	for _, required := range []string{"\n  temporal:", "\n  logstash:"} {
		if !strings.Contains(environmentCompose, required) {
			t.Errorf("environment Compose is missing %q", strings.TrimSpace(required))
		}
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func readArchitectureFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
