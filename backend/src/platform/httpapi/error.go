package httpapi

import (
	"errors"
	"fmt"
	"net/http"
)

type HTTPStatus int

const (
	StatusOK                  HTTPStatus = http.StatusOK
	StatusCreated             HTTPStatus = http.StatusCreated
	StatusAccepted            HTTPStatus = http.StatusAccepted
	StatusNoContent           HTTPStatus = http.StatusNoContent
	StatusBadRequest          HTTPStatus = http.StatusBadRequest
	StatusUnauthorized        HTTPStatus = http.StatusUnauthorized
	StatusForbidden           HTTPStatus = http.StatusForbidden
	StatusNotFound            HTTPStatus = http.StatusNotFound
	StatusConflict            HTTPStatus = http.StatusConflict
	StatusUnprocessableEntity HTTPStatus = http.StatusUnprocessableEntity
	StatusTooManyRequests     HTTPStatus = http.StatusTooManyRequests
	StatusInternalServerError HTTPStatus = http.StatusInternalServerError
	StatusServiceUnavailable  HTTPStatus = http.StatusServiceUnavailable
)

func (s HTTPStatus) Int() int { return int(s) }

type ErrorCode string

const (
	CodeInvalidJSON           ErrorCode = "invalid_json"
	CodeInvalidID             ErrorCode = "invalid_id"
	CodeUnauthorized          ErrorCode = "unauthorized"
	CodeForbidden             ErrorCode = "forbidden"
	CodeNotFound              ErrorCode = "not_found"
	CodeConflict              ErrorCode = "conflict"
	CodeValidationFailed      ErrorCode = "validation_failed"
	CodeRateLimited           ErrorCode = "rate_limited"
	CodeDependencyUnavailable ErrorCode = "dependency_unavailable"
	CodeSchemaUnavailable     ErrorCode = "schema_unavailable"
	CodeInternalError         ErrorCode = "internal_error"
	CodeRequestFailed         ErrorCode = "request_failed"
	CodeGenerationPlanInvalid ErrorCode = "generation_plan_invalid"
	CodeSessionInvalid        ErrorCode = "session_invalid"
	CodeWorkspaceInvalid      ErrorCode = "workspace_invalid"
	CodeProjectInvalid        ErrorCode = "project_invalid"
	CodeScriptInvalid         ErrorCode = "script_invalid"
)

type RecoveryAction struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type APIError struct {
	Status            HTTPStatus       `json:"-"`
	Code              ErrorCode        `json:"code"`
	Message           string           `json:"message"`
	NextAction        string           `json:"next_action"`
	Details           any              `json:"details,omitempty"`
	RequestID         string           `json:"request_id,omitempty"`
	RecoveryActions   []RecoveryAction `json:"recovery_actions,omitempty"`
	RetryAfterSeconds int              `json:"retry_after_seconds,omitempty"`
	Cause             error            `json:"-"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Cause)
	}
	return string(e.Code) + ": " + e.Message
}

func (e *APIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewError(status HTTPStatus, code ErrorCode, message, nextAction string) *APIError {
	return &APIError{Status: status, Code: code, Message: message, NextAction: nextAction}
}

func Wrap(err error, status HTTPStatus, code ErrorCode, message, nextAction string) *APIError {
	return &APIError{Status: status, Code: code, Message: message, NextAction: nextAction, Cause: err}
}

func NotFound(resource string) *APIError {
	return NewError(StatusNotFound, CodeNotFound, resource+"不存在", "确认资源 ID 和当前工作区后重试")
}

func Validation(message, nextAction string) *APIError {
	return NewError(StatusUnprocessableEntity, CodeValidationFailed, message, nextAction)
}

func Conflict(message, nextAction string) *APIError {
	return NewError(StatusConflict, CodeConflict, message, nextAction)
}

func Forbidden(message, nextAction string) *APIError {
	return NewError(StatusForbidden, CodeForbidden, message, nextAction)
}

func RateLimited(retryAfterSeconds int, message, nextAction string) *APIError {
	if retryAfterSeconds < 1 {
		retryAfterSeconds = 1
	}
	return &APIError{Status: StatusTooManyRequests, Code: CodeRateLimited, Message: message, NextAction: nextAction, RetryAfterSeconds: retryAfterSeconds}
}

func From(err error) *APIError {
	if err == nil {
		return nil
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	return Wrap(err, StatusInternalServerError, CodeInternalError, "服务暂时不可用", "稍后重试；如问题持续请提供 request_id")
}
