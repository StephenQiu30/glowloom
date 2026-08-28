package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/bootstrap"
	"github.com/StephenQiu30/lanverse/backend/internal/telemetry"
)

const backendStartupTimeout = 2 * time.Minute

func main() {
	logger, closeLogger, err := telemetry.NewLogstashLogger(
		os.Stdout, "lanverse-backend", os.Getenv("ENVIRONMENT"), os.Getenv("LOGSTASH_ADDRESS"),
	)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "lanverse backend logging configuration failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = closeLogger.Close() }()

	if len(os.Args) > 1 {
		bootstrap.RunEventWorker(logger)
		return
	}

	apiStopped := make(chan struct{})
	go func() {
		bootstrap.RunAPI(logger)
		close(apiStopped)
	}()
	if err = waitForAPI(apiStopped); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "lanverse backend startup failed: %v\n", err)
		os.Exit(1)
	}
	go bootstrap.RunWorkflowWorker(logger)
	go bootstrap.RunEventWorker(logger)
	<-apiStopped
}

func waitForAPI(apiStopped <-chan struct{}) error {
	deadline := time.NewTimer(backendStartupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	client := &http.Client{Timeout: time.Second}
	for {
		select {
		case <-apiStopped:
			return fmt.Errorf("API stopped before readiness")
		case <-deadline.C:
			return fmt.Errorf("API readiness timed out")
		case <-ticker.C:
			response, err := client.Get(apiReadinessURL())
			if err == nil {
				_ = response.Body.Close()
				if response.StatusCode == http.StatusOK {
					return nil
				}
			}
		}
	}
}

func apiReadinessURL() string {
	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8686"
	}
	return "http://" + net.JoinHostPort("127.0.0.1", port) + "/readyz"
}
