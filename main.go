package main

import (
	"context"
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

func main() {
	// Display startup banner
	showBanner()

	level := logger.ParseLevelFromEnv("LOG_LEVEL", logger.InfoLevel)
	log := logger.New(logger.Config{
		Level:          level,
		ServiceName:    "herald-dingtalk",
		ServiceVersion: version.Version,
	})

	port := config.Port
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}
	if !config.Valid() {
		log.Warn().Msg("DINGTALK_APP_KEY / DINGTALK_APP_SECRET / DINGTALK_AGENT_ID not set; /v1/send will return 503")
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: false})
	router.Setup(app, log)

	go func() {
		if err := app.Listen(port); err != nil {
			log.Fatal().Err(err).Msg("listen failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Info().Msg("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Warn().Err(err).Msg("shutdown error")
	}
}
