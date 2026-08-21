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
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/automation"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/config"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/coreipc"
	"github.com/CCdeAIHUB/WEBADBControl/server/internal/device"
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
	transport, err := coreipc.StartProcess(processContext, applicationConfig.CoreBinary)
	if err != nil {
		logger.Error("core_start_failed", "error", err)
		os.Exit(1)
	}
	defer transport.Close()
	core := coreipc.NewClient(transport)
	devices := device.NewService(core)
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
	settingsStore, err := settings.Open(filepath.Join(applicationConfig.DataDir, "settings.json"))
	if err != nil {
		logger.Error("settings_open_failed", "error", err)
		os.Exit(1)
	}
	handler := api.New(applicationConfig, devices, automationService, settingsStore, ai.NewService(settingsStore), logger).Handler()
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
	_ = httpServer.Shutdown(shutdownContext)
}
