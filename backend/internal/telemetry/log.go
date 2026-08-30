package telemetry

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"regexp"
	"strings"
)

const LogSchemaVersion = "lanverse.log.application"

var (
	urlValuePattern         = regexp.MustCompile(`(?i)https?://[^\s"']+`)
	secretAssignmentPattern = regexp.MustCompile(`(?i)(authorization|token|password|secret|credential|claim|prompt|candidate)(\s*[:=]\s*)([^\s,;]+)`)
	bearerPattern           = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`)
	jwtPattern              = regexp.MustCompile(`[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)
)

// NewLogger is the only application JSON logger constructor. It attaches the
// stable log schema and removes sensitive attributes before any log transport.
func NewLogger(writer io.Writer, service, environment string) *slog.Logger {
	if writer == nil {
		writer = io.Discard
	}
	service = boundedLogDimension(service, "unknown-service")
	environment = boundedLogDimension(environment, "development")
	handler := &redactingHandler{next: slog.NewJSONHandler(writer, nil)}
	return slog.New(handler).With(
		"schema_version", LogSchemaVersion,
		"service", service,
		"environment", environment,
	)
}

type redactingHandler struct {
	next          slog.Handler
	suppressAttrs bool
}

func (handler *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return handler.next.Enabled(ctx, level)
}

func (handler *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	clean := slog.NewRecord(record.Time, record.Level, sanitizeLogText(record.Message), record.PC)
	if !handler.suppressAttrs {
		record.Attrs(func(attribute slog.Attr) bool {
			if sanitized, ok := sanitizeAttribute(attribute, 0); ok {
				clean.AddAttrs(sanitized)
			}
			return true
		})
	}
	return handler.next.Handle(ctx, clean)
}

func (handler *redactingHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	if handler.suppressAttrs {
		return handler
	}
	clean := make([]slog.Attr, 0, len(attributes))
	for _, attribute := range attributes {
		if sanitized, ok := sanitizeAttribute(attribute, 0); ok {
			clean = append(clean, sanitized)
		}
	}
	return &redactingHandler{next: handler.next.WithAttrs(clean)}
}

func (handler *redactingHandler) WithGroup(name string) slog.Handler {
	if handler.suppressAttrs || sensitiveLogKey(name) {
		return &redactingHandler{next: handler.next, suppressAttrs: true}
	}
	return &redactingHandler{next: handler.next.WithGroup(name)}
}

func sanitizeAttribute(attribute slog.Attr, depth int) (slog.Attr, bool) {
	attribute.Value = attribute.Value.Resolve()
	if attribute.Equal(slog.Attr{}) || sensitiveLogKey(attribute.Key) || depth > 8 {
		return slog.Attr{}, false
	}
	switch attribute.Value.Kind() {
	case slog.KindString:
		attribute.Value = slog.StringValue(sanitizeLogText(attribute.Value.String()))
	case slog.KindAny:
		attribute.Value = slog.AnyValue(sanitizeLogValue(attribute.Value.Any(), depth+1))
	case slog.KindGroup:
		group := attribute.Value.Group()
		clean := make([]slog.Attr, 0, len(group))
		for _, nested := range group {
			if sanitized, ok := sanitizeAttribute(nested, depth+1); ok {
				clean = append(clean, sanitized)
			}
		}
		attribute.Value = slog.GroupValue(clean...)
	}
	return attribute, true
}

func sanitizeLogValue(value any, depth int) any {
	if depth > 8 || value == nil {
		return nil
	}
	if err, ok := value.(error); ok {
		return sanitizeLogText(err.Error())
	}
	if text, ok := value.(string); ok {
		return sanitizeLogText(text)
	}
	return sanitizeReflectedLogValue(reflect.ValueOf(value), depth)
}

func sanitizeReflectedLogValue(reflected reflect.Value, depth int) any {
	if depth > 8 || !reflected.IsValid() {
		return nil
	}
	for reflected.Kind() == reflect.Interface || reflected.Kind() == reflect.Pointer {
		if reflected.IsNil() {
			return nil
		}
		reflected = reflected.Elem()
		depth++
		if depth > 8 {
			return nil
		}
	}
	if !reflected.IsValid() {
		return nil
	}
	switch reflected.Kind() {
	case reflect.Map:
		clean := make(map[string]any)
		iterator := reflected.MapRange()
		for iterator.Next() {
			if iterator.Key().Kind() != reflect.String {
				continue
			}
			key := iterator.Key().String()
			if sensitiveLogKey(key) {
				continue
			}
			clean[key] = sanitizeReflectedLogValue(iterator.Value(), depth+1)
		}
		return clean
	case reflect.Slice, reflect.Array:
		if reflected.Type().Elem().Kind() == reflect.Uint8 {
			return "[REDACTED_BINARY]"
		}
		clean := make([]any, 0, reflected.Len())
		for index := 0; index < reflected.Len(); index++ {
			clean = append(clean, sanitizeReflectedLogValue(reflected.Index(index), depth+1))
		}
		return clean
	case reflect.Struct:
		clean := make(map[string]any)
		structure := reflected.Type()
		for index := 0; index < reflected.NumField(); index++ {
			field := structure.Field(index)
			value := reflected.Field(index)
			if field.PkgPath != "" || !value.CanInterface() {
				continue
			}
			key := field.Name
			if tag := strings.Split(field.Tag.Get("json"), ",")[0]; tag == "-" {
				continue
			} else if tag != "" {
				key = tag
			}
			if sensitiveLogKey(key) {
				continue
			}
			clean[key] = sanitizeReflectedLogValue(value, depth+1)
		}
		return clean
	case reflect.String:
		return sanitizeLogText(reflected.String())
	case reflect.Bool:
		return reflected.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflected.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return reflected.Uint()
	case reflect.Float32, reflect.Float64:
		return reflected.Float()
	default:
		return nil
	}
}

func sensitiveLogKey(key string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(strings.TrimSpace(key)))
	for _, forbidden := range []string{
		"authorization", "token", "password", "secret", "credential", "executiongrant",
		"apikey", "cookie", "sessionid", "claimtoken", "prompt", "candidate", "script",
		"scripttext", "rawscript", "sourcetext",
		"privateurl", "presignedurl", "artifacturl",
	} {
		if strings.Contains(normalized, forbidden) {
			return true
		}
	}
	return false
}

func sanitizeLogText(value string) string {
	value = urlValuePattern.ReplaceAllString(value, "[REDACTED_URL]")
	value = secretAssignmentPattern.ReplaceAllString(value, "$1$2[REDACTED]")
	value = bearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
	return jwtPattern.ReplaceAllString(value, "[REDACTED_TOKEN]")
}

func boundedLogDimension(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if len(value) > 64 {
		return value[:64]
	}
	return value
}
