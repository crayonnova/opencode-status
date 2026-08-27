package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/nova/opencode-status/internal/api"
	"github.com/nova/opencode-status/internal/config"
	"github.com/nova/opencode-status/internal/fetcher"
	"github.com/nova/opencode-status/internal/poller"
	"github.com/nova/opencode-status/internal/storage"
	"github.com/nova/opencode-status/internal/tui"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("opencode-status: ")

	cfg, err := config.FromArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	// Ensure DB parent dir exists.
	if cfg.DBPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
			log.Fatalf("mkdir db dir: %v", err)
		}
	}

	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer store.Close()

	f := fetcher.New(cfg.ModelsDevURL, cfg.OpenRouterURL)
	p := &poller.Poller{
		Store:       store,
		Fetcher:     f,
		Interval:    cfg.PollInterval,
		IncludePaid: cfg.ShowPaid,
		Logger:      log.Default(),
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Background poller.
	pollerDone := make(chan struct{})
	go func() {
		defer close(pollerDone)
		_ = p.Run(ctx)
	}()

	// Optional HTTP server.
	if !cfg.WebDisable && cfg.WebAddr != "" {
		srv := api.New(store, cfg.WebAddr, time.Duration(cfg.RetentionDays)*24*time.Hour)
		go func() {
			log.Printf("http listening on %s", cfg.WebAddr)
			if err := srv.Start(); err != nil && err.Error() != "http: Server closed" {
				log.Printf("http: %v", err)
			}
		}()
		defer func() {
			shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutCancel()
			_ = srv.Shutdown(shutCtx)
		}()
	}

	// Prune goroutine (daily).
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := store.Prune(time.Duration(cfg.RetentionDays) * 24 * time.Hour); err == nil {
					log.Printf("pruned %d old rows", n)
				}
			}
		}
	}()

	if cfg.TUIDisable {
		log.Printf("tui disabled, running as daemon")
		<-ctx.Done()
	} else {
		if err := tui.Run(ctx, store, cfg.PollInterval); err != nil && err != context.Canceled {
			log.Printf("tui: %v", err)
		}
		cancel()
	}

	<-pollerDone
	log.Printf("bye")
}
