package platform_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func TestStrictJSONRejectsTrailingGarbageAndOversizedSuffix(t *testing.T) {
	t.Parallel()
	for _, suffix := range []string{"garbage", "{", "{}", strings.Repeat(" ", 1<<20)} {
		r := httptest.NewRequest("POST", "/test", strings.NewReader(`{"title":"测试"}`+suffix))
		w := httptest.NewRecorder()
		var value struct {
			Title string `json:"title"`
		}
		if platformhttp.DecodeStrict(w, r, nil, &value) {
			t.Fatal("accepted invalid trailing body")
		}
	}
}
