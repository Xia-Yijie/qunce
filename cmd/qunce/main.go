package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := loadConfig()
	st, err := newState(cfg)
	if err != nil {
		return err
	}

	agentRegistry := newAgentRegistry()
	consoleRegistry := newConsoleRegistry()
	ctx := newHandlerContext(st, agentRegistry, consoleRegistry, cfg)

	mux := http.NewServeMux()
	registerRoutes(mux, ctx)
	registerAgentSocket(mux, st, agentRegistry, consoleRegistry, cfg)
	registerConsoleSocket(mux, consoleRegistry)
	registerStaticRoutes(mux)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("qunce server listening on %s", addr)
		errCh <- server.ListenAndServe()
	}()

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-shutdownCh:
		log.Printf("shutdown by signal: %v", sig)
		ctxTimeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(ctxTimeout)
	case err := <-errCh:
		return err
	}
}
