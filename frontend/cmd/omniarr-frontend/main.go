package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/angoo/omniarr/frontend/internal/auth"
	"github.com/angoo/omniarr/frontend/internal/config"
	"github.com/angoo/omniarr/frontend/internal/handlers"
	"github.com/angoo/omniarr/frontend/internal/proxy"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	configPath := flag.String("config", "frontend-config.yaml", "Path to configuration file")
	flag.Parse()

	// Load Configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Configure Logger
	var level slog.Level
	switch cfg.Server.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)

	// Initialize Auth Manager
	authManager, err := auth.NewManager(context.Background(), cfg.Auth)
	if err != nil {
		slog.Error("Failed to initialize auth manager", "error", err)
		os.Exit(1)
	}

	// Initialize Backend Proxy Client
	proxyClient := proxy.NewClient(cfg.Backends)

	// Initialize Handlers
	h, err := handlers.NewHandler(authManager, proxyClient, cfg)
	if err != nil {
		slog.Error("Failed to initialize handlers", "error", err)
		os.Exit(1)
	}

	// Setup Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Public Routes
	r.Get("/login", h.Login)
	r.Get("/login/oidc", h.LoginOIDC)
	r.Get("/auth/callback", h.AuthCallback)

	// Protected Routes
	r.Group(func(r chi.Router) {
		r.Use(authManager.Middleware)
		
		r.Get("/", h.Index)
		r.Get("/logout", h.Logout)
		r.Get("/view/{backendID}", h.ViewBackend)
		r.Get("/view/{backendID}/search", h.SearchPage)
		r.Get("/view/{backendID}/details", h.GetEntityDetails)
		
		// Admin Routes
		r.Get("/admin", h.Admin)
		r.Delete("/admin/entity/{backendID}/{uuid}", h.DeleteEntity)

		// HTMX Actions
		r.Post("/api/{backendID}/search", h.Search)
		r.Post("/api/{backendID}/request", h.Request)
	})

	// Start Server
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		slog.Info("Starting OmniArr Frontend", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	slog.Info("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("Server exited properly")
}