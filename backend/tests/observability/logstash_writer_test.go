package observability_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/telemetry"
)

func TestLogstashLoggerWritesTheSameRedactedJSONToStdoutAndTCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan []byte, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		line, readErr := bufio.NewReader(connection).ReadBytes('\n')
		if readErr == nil {
			received <- line
		}
	}()

	var stdout bytes.Buffer
	logger, closer, err := telemetry.NewLogstashLogger(
		&stdout, "lanverse-backend", "test", listener.Addr().String(),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	logger.Info("token=must-not-leak", "request_id", "request-1", "password", "must-not-leak")

	var remote []byte
	select {
	case remote = <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("Logstash TCP endpoint did not receive a log record")
	}
	for name, encoded := range map[string][]byte{"stdout": stdout.Bytes(), "logstash": remote} {
		var record map[string]any
		if err = json.Unmarshal(encoded, &record); err != nil {
			t.Fatalf("%s log is not JSON: %v", name, err)
		}
		if record["schema_version"] != telemetry.LogSchemaVersion || record["request_id"] != "request-1" {
			t.Fatalf("%s log lost the stable schema: %#v", name, record)
		}
		if bytes.Contains(encoded, []byte("must-not-leak")) || bytes.Contains(encoded, []byte("password")) {
			t.Fatalf("%s log leaked a secret: %s", name, encoded)
		}
	}
}

func TestLogstashLoggerRejectsInvalidAddressAndFailsOpenWhenEndpointIsUnavailable(t *testing.T) {
	if _, _, err := telemetry.NewLogstashLogger(&bytes.Buffer{}, "lanverse-backend", "test", "missing-port"); err == nil {
		t.Fatal("invalid Logstash address was accepted")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	logger, closer, err := telemetry.NewLogstashLogger(&stdout, "lanverse-backend", "test", address)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	logger.Info("service stays available", "request_id", "request-2")
	if !bytes.Contains(stdout.Bytes(), []byte("request-2")) {
		t.Fatalf("stdout logging failed with Logstash unavailable: %s", stdout.Bytes())
	}
}
