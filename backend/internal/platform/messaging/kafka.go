package messaging

import (
	"os"
	"strings"
)

const defaultBrokers = "127.0.0.1:9092"
const ScriptAnalysisTopic = "lanverse.script-analysis.requested"

func Brokers() []string {
	value := os.Getenv("KAFKA_BROKERS")
	if value == "" {
		value = defaultBrokers
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
