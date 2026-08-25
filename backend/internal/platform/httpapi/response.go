package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	platformvalidation "github.com/StephenQiu30/lanverse/backend/internal/platform/validation"
)

const maxJSONBodyBytes = 1 << 20

type Problem struct {
	Code, Message, NextAction string
	Status                    int
	Details                   map[string]any
}

func DecodeStrict(writer http.ResponseWriter, request *http.Request, validator *platformvalidation.Validator, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxJSONBodyBytes))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		WriteProblem(writer, request, Problem{Code: "validation_failed", Message: "Request validation failed", Status: 422})
		return false
	}
	var extra any
	if decoder.Decode(&extra) == nil {
		WriteProblem(writer, request, Problem{Code: "validation_failed", Message: "Request validation failed", Status: 422})
		return false
	}
	if validator != nil && validator.Struct(target) != nil {
		WriteProblem(writer, request, Problem{Code: "validation_failed", Message: "Request validation failed", Status: 422})
		return false
	}
	return true
}

func WriteProblem(writer http.ResponseWriter, request *http.Request, problem Problem) {
	if problem.Details == nil {
		problem.Details = map[string]any{}
	}
	WriteJSON(writer, problem.Status, map[string]any{"error": map[string]any{
		"code": problem.Code, "message": problem.Message,
		"request_id":  nullable(request.Header.Get("X-Request-ID")),
		"next_action": nullable(problem.NextAction), "details": problem.Details,
	}})
}

func WriteJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
