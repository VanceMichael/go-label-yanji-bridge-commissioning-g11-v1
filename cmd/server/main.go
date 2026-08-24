package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/config"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/httpapi"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/idgen"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/service"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/storage/sqlite"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("bridgewatch stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	store, err := sqlite.Open(rootCtx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer store.Close()
	now := time.Now().UTC()
	users := []sqlite.BootstrapUser{{ID: "user_owner", Email: "owner@bridgewatch.local", DisplayName: "Owner Admin", Role: domain.RoleOwnerAdmin, Password: cfg.BootstrapPassword}, {ID: "user_contractor", Email: "contractor@bridgewatch.local", DisplayName: "Contractor Engineer", Role: domain.RoleContractorEngineer, Password: cfg.BootstrapPassword}, {ID: "user_supervisor", Email: "supervisor@bridgewatch.local", DisplayName: "Supervision Engineer", Role: domain.RoleSupervisor, Password: cfg.BootstrapPassword}, {ID: "user_commissioning", Email: "commissioning@bridgewatch.local", DisplayName: "Commissioning Officer", Role: domain.RoleCommissioning, Password: cfg.BootstrapPassword}}
	if err := store.Bootstrap(rootCtx, "org_yanji", "Yanji Bridge Commissioning Command", users, now); err != nil {
		return err
	}
	svc := service.New(store, repository.RealClock{})
	runner := worker.New(store, idgen.New("worker"), cfg.WorkerInterval, logger)
	worker.RegisterBridgeHandlers(runner, svc)
	workerDone := make(chan error, 1)
	go func() { workerDone <- runner.Run(rootCtx) }()
	httpServer := &http.Server{Addr: cfg.Address, Handler: httpapi.New(svc, cfg.SessionTTL, cfg.MaxRequestBytes, logger), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	serverDone := make(chan error, 1)
	go func() {
		logger.Info("bridgewatch listening", "address", cfg.Address)
		serverDone <- httpServer.ListenAndServe()
	}()
	select {
	case err := <-serverDone:
		if !errors.Is(err, http.ErrServerClosed) {
			stop()
			return err
		}
	case err := <-workerDone:
		if !errors.Is(err, context.Canceled) {
			stop()
			return err
		}
	case <-rootCtx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}
