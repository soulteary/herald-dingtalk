package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pterm/pterm"
	"github.com/pterm/pterm/putils"
	"github.com/soulteary/herald-dingtalk/internal/config"
	"github.com/soulteary/herald-dingtalk/internal/router"
	"github.com/soulteary/logger-kit"
	version "github.com/soulteary/version-kit"
)

const shutdownTimeout = 10 * time.Second

var listenTCP = net.Listen

// showBanner displays the startup banner with version
func showBanner() {
	pterm.DefaultBox.Println(
		putils.CenterText(
			"Herald DingTalk\n" +
				"DingTalk Notification Provider for Herald\n" +
				"Version: " + version.Version,
		),
	)
	time.Sleep(time.Millisecond) // Don't ask why, but this fixes the docker-compose log
}

func newApp() *fiber.App {
	return fiber.New(fiber.Config{
		DisableStartupMessage: false,
		BodyLimit:             config.EffectiveMaxRequestBodyBytes(),
		ReadTimeout:           10 * time.Second,
		WriteTimeout:          35 * time.Second,
		IdleTimeout:           60 * time.Second,
	})
}

func listenAddress(port string) string {
	if !strings.HasPrefix(port, ":") {
		return ":" + port
	}
	return port
}

func run(log *logger.Logger, quit <-chan os.Signal) error {
	if err := config.Validate(); err != nil {
		log.Warn().Err(err).Msg("invalid DingTalk configuration; /v1/send and /v1/resolve will return 503")
	}
	app := newApp()
	router.Setup(app, log)
	listener, err := listenTCP("tcp", listenAddress(config.Port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer func() { _ = listener.Close() }()
	return serve(app, listener, quit, shutdownTimeout, log)
}

func serve(app *fiber.App, listener net.Listener, quit <-chan os.Signal, timeout time.Duration, log *logger.Logger) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- app.Listener(listener) }()

	select {
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return fmt.Errorf("serve: server stopped unexpectedly")
	case <-quit:
		log.Info().Msg("shutting down")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := app.ShutdownWithContext(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; err != nil {
		return fmt.Errorf("serve after shutdown: %w", err)
	}
	return nil
}

func main() {
	showBanner()
	level := logger.ParseLevelFromEnv("LOG_LEVEL", logger.InfoLevel)
	log := logger.New(logger.Config{
		Level:          level,
		ServiceName:    "herald-dingtalk",
		ServiceVersion: version.Version,
	})
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(quit)
	if err := run(log, quit); err != nil {
		log.Error().Err(err).Msg("server stopped")
		os.Exit(1)
	}
}
