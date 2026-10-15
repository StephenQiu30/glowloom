package operation_test

import (
	"encoding/json"
	"testing"

	operationhttp "github.com/StephenQiu30/glowloom-server/backend/internal/operation/adapter/http"
)

func TestEditorHTTPRejectsMixedInlineIntentEvenWhenEmpty(t *testing.T) {
	pointer := `"input_source":{"kind":"prompt_draft","draft_id":"11111111-1111-4111-8111-111111111111","revision":1},"expected_submission_epoch":0`
	for _, field := range []string{`"prompt":""`, `"params":null`, `"output_count":0`, `"source":null`, `"media_inputs":[]`, `"prompt_template":null`} {
		var body operationhttp.QuoteItemRequest
		if err := json.Unmarshal([]byte(`{`+pointer+`,`+field+`}`), &body); err == nil {
			t.Fatal("mixed fields accepted", field)
		}
	}
	var body operationhttp.QuoteItemRequest
	if err := json.Unmarshal([]byte(`{`+pointer+`}`), &body); err != nil {
		t.Fatal(err)
	}
}
