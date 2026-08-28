package telemetry

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	logstashNetworkTimeout = 500 * time.Millisecond
	logstashRetryInterval  = 5 * time.Second
)

// NewLogstashLogger creates the single Backend logger. Stdout remains the
// durable local diagnostic path; a configured Logstash endpoint receives the
// same already-redacted JSON record without becoming a service dependency.
func NewLogstashLogger(
	stdout io.Writer,
	service string,
	environment string,
	address string,
) (*slog.Logger, io.Closer, error) {
	if stdout == nil {
		stdout = io.Discard
	}
	address = strings.TrimSpace(address)
	if address == "" {
		return NewLogger(stdout, service, environment), noOpCloser{}, nil
	}
	if err := validateLogstashAddress(address); err != nil {
		return nil, nil, err
	}
	remote := &logstashWriter{address: address}
	return NewLogger(io.MultiWriter(stdout, remote), service, environment), remote, nil
}

type noOpCloser struct{}

func (noOpCloser) Close() error { return nil }

type logstashWriter struct {
	address  string
	mu       sync.Mutex
	conn     net.Conn
	nextDial time.Time
}

func (writer *logstashWriter) Write(record []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	now := time.Now()
	if writer.conn == nil {
		if now.Before(writer.nextDial) {
			return len(record), nil
		}
		connection, err := net.DialTimeout("tcp", writer.address, logstashNetworkTimeout)
		if err != nil {
			writer.nextDial = now.Add(logstashRetryInterval)
			return len(record), nil
		}
		writer.conn = connection
		writer.nextDial = time.Time{}
	}

	if err := writer.conn.SetWriteDeadline(now.Add(logstashNetworkTimeout)); err != nil {
		writer.disconnect(now)
		return len(record), nil
	}
	written, err := writer.conn.Write(record)
	if err != nil || written != len(record) {
		writer.disconnect(now)
	}
	return len(record), nil
}

func (writer *logstashWriter) Close() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.conn == nil {
		return nil
	}
	err := writer.conn.Close()
	writer.conn = nil
	return err
}

func (writer *logstashWriter) disconnect(now time.Time) {
	_ = writer.conn.Close()
	writer.conn = nil
	writer.nextDial = now.Add(logstashRetryInterval)
}

func validateLogstashAddress(address string) error {
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(host) == "" {
		return fmt.Errorf("LOGSTASH_ADDRESS must use host:port")
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("LOGSTASH_ADDRESS must use a port between 1 and 65535")
	}
	return nil
}
