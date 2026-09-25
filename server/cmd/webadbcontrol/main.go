package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/ai"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/api"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/auth"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/automation"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/config"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/coreipc"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/observability"
	adbRuntime "github.com/CCdeAIHUB/WEBADBControl/server/internal/runtime"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/settings"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	applicationConfig, err := config.Load()
	if err != nil {
		logger.Error("configuration_failed", "error", err)
		os.Exit(1)
	}
	processContext, cancelProcess := context.WithCancel(context.Background())
	defer cancelProcess()
	transport, err := coreipc.StartProcessWithEnv(
		processContext,
		applicationConfig.CoreBinary,
		map[string]string{
			"ADBCONTROL_REMOTE_DATA_DIR": applicationConfig.CoreRemoteDataDir,
			"ADBCONTROL_REMOTE_LISTEN":   applicationConfig.CoreRemoteListen,
		},
	)
	if err != nil {
		logger.Error("core_start_failed", "error", err)
		os.Exit(1)
	}
	defer transport.Close()
	core := coreipc.NewClient(transport)
	devices := device.NewService(core, device.WithCompanionRequirement(device.CompanionRequirement{
		APKPath:     applicationConfig.CompanionAPK,
		VersionCode: applicationConfig.CompanionVersionCode,
		VersionName: applicationConfig.CompanionVersionName,
	}))
	devices.SetLogger(logger)
	repository, err := automation.OpenRepository(filepath.Join(applicationConfig.DataDir, "automation.sqlite"))
	if err != nil {
		logger.Error("automation_repository_failed", "error", err)
		os.Exit(1)
	}
	defer repository.Close()
	if err := repository.RecoverInterrupted(context.Background()); err != nil {
		logger.Error("automation_recovery_failed", "error", err)
		os.Exit(1)
	}
	automationService := automation.NewService(repository, adbRuntime.NewExecutor(devices))
	automationService.SetLogger(logger)
	logRepository, err := observability.OpenRepository(filepath.Join(applicationConfig.DataDir, "observability.sqlite"))
	if err != nil {
		logger.Error("observability_repository_failed", "error", err)
		os.Exit(1)
	}
	defer logRepository.Close()
	logService := observability.NewService(logRepository)
	if err := logService.Record(context.Background(), observability.Event{
		Type:    observability.TypeSystem,
		Level:   observability.LevelInfo,
		Module:  "server",
		Action:  "server.boot",
		Message: "服务进程启动",
		TraceID: "server-startup",
	}); err != nil {
		logger.Warn("observability_startup_log_failed", "error", err)
	}
	settingsStore, err := settings.Open(filepath.Join(applicationConfig.DataDir, "settings.json"))
	if err != nil {
		logger.Error("settings_open_failed", "error", err)
		os.Exit(1)
	}
	authenticator := auth.New(core)
	handler := api.New(applicationConfig, devices, automationService, settingsStore, ai.NewService(settingsStore), authenticator, logService, logger).Handler()
	httpServer := &http.Server{Addr: applicationConfig.Address, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}

	go func() {
		logger.Info("server_started", "address", applicationConfig.Address)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server_failed", "error", err)
			cancelProcess()
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-stop:
	case <-processContext.Done():
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelShutdown()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		logger.Error("server_shutdown_failed", "error", err)
	}
}
