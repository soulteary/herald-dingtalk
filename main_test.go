package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pterm/pterm"
	"github.com/soulteary/herald-dingtalk/internal/config"
	"github.com/soulteary/logger-kit/v2"
)

func TestShowBanner(t *testing.T) {
	outputEnabled := pterm.Output
	pterm.DisableOutput()
	t.Cleanup(func() {
		if outputEnabled {
			pterm.EnableOutput()
		}
	})
	showBanner()
}

func TestNewAppEnforcesBodyLimit(t *testing.T) {
	original := config.MaxRequestBodyBytes
	config.MaxRequestBodyBytes = 32
	t.Cleanup(func() { config.MaxRequestBodyBytes = original })

	app := newApp()
	app.Post("/echo", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) })
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(strings.Repeat("x", 33)))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := app.Test(req)
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		t.Fatalf("expected body limit error, got status %d", resp.StatusCode)
	}
	if !strings.Contains(err.Error(), "body size exceeds") {
		t.Fatalf("error = %q, want body size limit error", err)
	}
}

func TestListenAddress(t *testing.T) {
	for _, tt := range []struct{ input, want string }{
		{input: "8083", want: ":8083"},
		{input: ":8083", want: ":8083"},
	} {
		if got := listenAddress(tt.input); got != tt.want {
			t.Fatalf("listenAddress(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestServeWaitsForInFlightRequestBeforeShutdown(t *testing.T) {
	app := fiber.New()
	started := make(chan struct{})
	release := make(chan struct{})
	app.Get("/slow", func(c fiber.Ctx) error {
		close(started)
		<-release
		return c.SendStatus(fiber.StatusNoContent)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	quit := make(chan os.Signal, 1)
	var logs bytes.Buffer
	log := logger.New(logger.Config{Level: logger.InfoLevel, Output: &logs})
	serveDone := make(chan error, 1)
	go func() { serveDone <- serve(app, listener, quit, time.Second, log) }()

	client := &http.Client{Timeout: 2 * time.Second}
	requestDone := make(chan error, 1)
	go func() {
		resp, requestErr := client.Get("http://" + listener.Addr().String() + "/slow")
		if requestErr == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				requestErr = fmt.Errorf("status = %d, want 204", resp.StatusCode)
			}
		}
		requestDone <- requestErr
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request did not reach handler")
	}
	quit <- syscall.SIGTERM
	select {
	case err := <-serveDone:
		t.Fatalf("serve returned before in-flight request completed: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("serve did not stop")
	}
	if !strings.Contains(logs.String(), "shutting down") {
		t.Fatalf("shutdown log missing: %s", logs.String())
	}
	if _, err := client.Get("http://" + listener.Addr().String() + "/slow"); err == nil {
		t.Fatal("listener still accepts requests after shutdown")
	}
}

func TestServeReturnsListenerFailure(t *testing.T) {
	app := fiber.New()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	log := logger.New(logger.Config{Level: logger.Disabled, Output: io.Discard})
	err = serve(app, listener, nil, time.Second, log)
	if err == nil || !strings.Contains(err.Error(), "serve:") {
		t.Fatalf("error = %v, want listener serve failure", err)
	}
}

func TestRunServesUntilShutdownSignal(t *testing.T) {
	originalListen := listenTCP
	originalPort := config.Port
	originalAppKey, originalSecret := config.AppKey, config.AppSecret
	originalAgentID, originalLookupMode := config.AgentID, config.LookupMode
	t.Cleanup(func() {
		listenTCP = originalListen
		config.Port = originalPort
		config.AppKey, config.AppSecret = originalAppKey, originalSecret
		config.AgentID, config.LookupMode = originalAgentID, originalLookupMode
	})

	listenerReady := make(chan net.Listener, 1)
	listenTCP = func(_, _ string) (net.Listener, error) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err == nil {
			listenerReady <- listener
		}
		return listener, err
	}
	config.Port = "8083"
	config.AppKey, config.AppSecret, config.AgentID = "key", "secret", "1"
	config.LookupMode = config.LookupModeNone

	quit := make(chan os.Signal, 1)
	log := logger.New(logger.Config{Level: logger.Disabled, Output: io.Discard})
	runDone := make(chan error, 1)
	go func() { runDone <- run(log, quit) }()
	listener := <-listenerReady

	client := &http.Client{Timeout: time.Second}
	deadline := time.After(time.Second)
	for {
		resp, err := client.Get("http://" + listener.Addr().String() + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("health status = %d, want 200", resp.StatusCode)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatalf("server did not become ready: %v", err)
		default:
			time.Sleep(time.Millisecond)
		}
	}

	quit <- syscall.SIGTERM
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("run did not stop")
	}
}

func TestRunReturnsListenError(t *testing.T) {
	originalListen := listenTCP
	t.Cleanup(func() { listenTCP = originalListen })
	wantErr := errors.New("bind failed")
	listenTCP = func(_, _ string) (net.Listener, error) { return nil, wantErr }
	log := logger.New(logger.Config{Level: logger.Disabled, Output: io.Discard})
	err := run(log, make(chan os.Signal))
	if !errors.Is(err, wantErr) || !strings.Contains(err.Error(), "listen:") {
		t.Fatalf("run error = %v, want wrapped bind error", err)
	}
}
