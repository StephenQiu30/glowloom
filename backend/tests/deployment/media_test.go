package deployment_test

import (
	"slices"
	"strings"
	"testing"
)

func TestMediaProcessorIsPrivateIndependentAndCapacityBounded(t *testing.T) {
	for _, overlay := range []bool{false, true} {
		name := "ordinary"
		if overlay {
			name = "development"
		}
		t.Run(name, func(t *testing.T) {
			v := readDeployment(t, overlay)
			const prefix = "services.media."
			if got := v.GetString(prefix + "build.dockerfile"); got != "media.Dockerfile" {
				t.Errorf("Media build=%q", got)
			}
			if len(v.GetStringSlice(prefix+"ports")) != 0 || !slices.Equal(v.GetStringSlice(prefix+"expose"), []string{"9091", "8081"}) {
				t.Error("Media listeners must remain private")
			}
			if len(v.GetStringMap(prefix+"depends_on")) != 0 || len(v.GetStringSlice(prefix+"volumes")) != 0 {
				t.Error("Media depends on business infrastructure or shared files")
			}
			if !v.GetBool(prefix+"read_only") || v.GetString(prefix+"mem_limit") == "" || v.GetInt(prefix+"cpus") != 2 {
				t.Error("Media lacks runtime resource bounds")
			}
			if got := strings.Join(v.GetStringSlice(prefix+"tmpfs"), " "); !strings.Contains(got, "/work:rw,noexec,nosuid,size=2100m") {
				t.Errorf("unbounded or executable Media scratch: %s", got)
			}
			allowed := map[string]bool{"glowloom_media_env": true, "glowloom_media_grpc_addr": true, "glowloom_media_http_addr": true, "glowloom_media_service_token": true, "glowloom_media_concurrency": true}
			for key := range v.GetStringMap(prefix + "environment") {
				if !allowed[strings.ToLower(key)] {
					t.Errorf("Media received unrelated configuration: %s", key)
				}
			}
			if got := v.GetString("services.backend.environment.GLOWLOOM_MEDIA_NORMALIZATION_ADDRESS"); got != "media:9091" {
				t.Errorf("API remote target=%q", got)
			}
			if got := v.GetString("services.backend.depends_on.media.condition"); got != "service_healthy" {
				t.Errorf("API Media admission=%q", got)
			}
			for _, service := range []string{"gateway", "worker", "relay"} {
				if got := v.GetString("services." + service + ".environment.GLOWLOOM_MEDIA_SERVICE_TOKEN"); got != "" {
					t.Errorf("%s received Media credentials", service)
				}
				if v.IsSet("services." + service + ".depends_on.media") {
					t.Errorf("%s unnecessarily waits for Media", service)
				}
			}
			if overlay && !v.IsSet(prefix+"develop.watch") {
				t.Error("Media development build does not watch its own source")
			}
		})
	}
}
