// Command daytrader is the Go replacement for the DayTrader 3 EAR.
//
//	daytrader serve   HTTP server on LISTEN_ADDR (default; context path /daytrader)
//	daytrader seed    buildDBTables + buildDB against DATABASE_URL, then exit
//	daytrader migrate buildDBTables only
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/config"
	_ "github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/features/all"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/seed"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/store"
	"github.com/Cognition-Partner-Workshops/uc-java-ee-6-modernization/daytrader-go/internal/web"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if err := run(command, logger); err != nil {
		logger.Error("daytrader stopped", "command", command, "error", err)
		os.Exit(1)
	}
}

func run(command string, logger *slog.Logger) error {
	cfg, err := config.FromEnv(nil)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	db, err := store.Open(ctx, cfg.DatabaseURL, cfg.DBMaxConns, cfg.KeyBlockSize)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer db.Close()

	switch command {
	case "migrate":
		return seed.BuildTables(ctx, db)
	case "seed":
		started := time.Now()
		if err := seed.BuildTables(ctx, db); err != nil {
			return err
		}
		result, err := seed.BuildDB(ctx, db, cfg, nil)
		if err != nil {
			return err
		}
		logger.Info("seeded", "quotes", result.Quotes, "users", result.Users, "holdings", result.Holdings,
			"orders", result.Orders, "seconds", time.Since(started).Seconds())
		return nil
	case "serve":
		app, err := web.New(cfg, db, logger)
		if err != nil {
			return err
		}
		server := &http.Server{Addr: cfg.ListenAddr, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second}
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdownCtx)
		}()
		logger.Info("listening", "addr", cfg.ListenAddr, "contextPath", cfg.ContextPath,
			"orderProcessingMode", cfg.OrderProcessingMode, "actions", web.RegisteredActions())
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown command %q (serve, seed, migrate)", command)
	}
}
