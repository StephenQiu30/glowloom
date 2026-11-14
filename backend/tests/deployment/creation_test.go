package deployment_test

import (
	"slices"
	"strings"
	"testing"
)

func TestCreationExtractorIsPrivateIndependentAndCapacityBounded(t *testing.T) {
	for _, overlay := range []bool{false, true} {
		name := "ordinary"
		if overlay {
			name = "development"
		}
		t.Run(name, func(t *testing.T) {
			v := readDeployment(t, overlay)
			const prefix = "services.creation."
			if v.GetString(prefix+"build.dockerfile") != "creation.Dockerfile" {
				t.Error("Creation must use its own binary image")
			}
			if len(v.GetStringSlice(prefix+"ports")) != 0 || !slices.Equal(v.GetStringSlice(prefix+"expose"), []string{"9092", "8083"}) {
				t.Error("Creation listeners must remain private")
			}
			if len(v.GetStringMap(prefix+"depends_on")) != 0 || len(v.GetStringSlice(prefix+"volumes")) != 0 {
				t.Error("Creation cannot depend on business middleware or shared files")
			}
			if !v.GetBool(prefix+"read_only") || v.GetString(prefix+"mem_limit") == "" || v.GetInt(prefix+"cpus") != 2 {
				t.Error("Creation lacks runtime resource bounds")
			}
			if got := strings.Join(v.GetStringSlice(prefix+"tmpfs"), " "); !strings.Contains(got, "/work:rw,noexec,nosuid,size=64m") {
				t.Errorf("unbounded or executable scratch: %s", got)
			}
			allowed := map[string]bool{"glowloom_creation_env": true, "glowloom_creation_grpc_addr": true, "glowloom_creation_http_addr": true, "glowloom_creation_service_token": true, "glowloom_creation_concurrency": true}
			for key := range v.GetStringMap(prefix + "environment") {
				if !allowed[strings.ToLower(key)] {
					t.Errorf("Creation received unrelated configuration: %s", key)
				}
			}
			for _, service := range []string{"backend", "worker"} {
				if got := v.GetString("services." + service + ".environment.GLOWLOOM_CREATION_EXTRACTION_ADDRESS"); got != "creation:9092" {
					t.Errorf("%s target=%q", service, got)
				}
				if got := v.GetString("services." + service + ".depends_on.creation.condition"); got != "service_healthy" {
					t.Errorf("%s Creation admission=%q", service, got)
				}
			}
			for _, service := range []string{"gateway", "relay", "agent", "canvas"} {
				if got := v.GetString("services." + service + ".environment.GLOWLOOM_CREATION_SERVICE_TOKEN"); got != "" {
					t.Errorf("%s received Creation identity", service)
				}
				if v.IsSet("services." + service + ".depends_on.creation") {
					t.Errorf("%s unnecessarily waits for Creation", service)
				}
			}
			if overlay && !v.IsSet(prefix+"develop.watch") {
				t.Error("Creation development build does not watch its own code")
			}
		})
	}
}
