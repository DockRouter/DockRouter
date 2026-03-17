// Package main is the entry point for DockRouter
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/DockRouter/dockrouter/internal/admin"
	"github.com/DockRouter/dockrouter/internal/config"
	"github.com/DockRouter/dockrouter/internal/discovery"
	"github.com/DockRouter/dockrouter/internal/health"
	"github.com/DockRouter/dockrouter/internal/log"
	"github.com/DockRouter/dockrouter/internal/metrics"
	"github.com/DockRouter/dockrouter/internal/middleware"
	"github.com/DockRouter/dockrouter/internal/proxy"
	"github.com/DockRouter/dockrouter/internal/router"
	tlspkg "github.com/DockRouter/dockrouter/internal/tls"
)

// Build-time variables (set via ldflags)
var (
	version   = "dev"
	buildTime = "unknown"
)

// Embed dashboard files
//go:embed dashboard/*
var dashboardFS embed.FS

// App holds all application components
type App struct {
	config            *config.Config
	logger            *log.Logger
	routeTable        *router.Table
	tlsManager        *tlspkg.Manager
	challengeSolver   *tlspkg.ChallengeSolver
	healthChecker     *health.Checker
	discoveryEngine   *discovery.Engine
	metrics           *metrics.Collector
	middlewareBuilder *router.RouteMiddlewareBuilder
	startTime         time.Time
}

func main() {
	// Load configuration
	cfg, err := config.Load(version, buildTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := log.NewLogger(os.Stdout, parseLogLevel(cfg.LogLevel))

	logger.Info("DockRouter starting",
		"version", cfg.Version,
		"http_port", cfg.HTTPPort,
		"https_port", cfg.HTTPSPort,
		"admin", cfg.Admin,
	)

	// Create app
	app := &App{
		config:    cfg,
		logger:    logger,
		startTime: time.Now(),
	}

	// Initialize components
	if err := app.initialize(); err != nil {
		logger.Fatal("Failed to initialize", "error", err)
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start servers
	app.start(ctx)

	logger.Info("DockRouter ready",
		"http", fmt.Sprintf(":%d", cfg.HTTPPort),
		"https", fmt.Sprintf(":%d", cfg.HTTPSPort),
		"routes", app.routeTable.Count(),
	)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down...")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	app.shutdown(shutdownCtx)
	cancel()

	logger.Info("Goodbye!")
}

func (a *App) initialize() error {
	// Initialize metrics
	a.metrics = metrics.NewCollector()

	// Initialize route table
	a.routeTable = router.NewTable()

	// Initialize health checker
	a.healthChecker = health.NewChecker(10*time.Second, 5*time.Second)

	// Initialize challenge solver
	a.challengeSolver = tlspkg.NewChallengeSolver()

	// Initialize TLS components
	if a.config.ACMEEmail != "" {
		tlsStore := tlspkg.NewStore(a.config.DataDir)
		acmeClient := tlspkg.NewACMEClient(a.config.GetACMEDirectoryURL(), a.config.ACMEEmail)

		if err := acmeClient.Initialize(); err != nil {
			a.logger.Warn("Failed to initialize ACME client", "error", err)
		}

		a.tlsManager = tlspkg.NewManager(tlsStore, acmeClient, a.challengeSolver, a.logger)

		// Load existing certificates
		if err := a.tlsManager.LoadFromDisk(); err != nil {
			a.logger.Warn("Failed to load certificates", "error", err)
		}
	}

	// Initialize Docker discovery
	dockerClient, err := discovery.NewDockerClient(a.config.DockerSocket)
	if err != nil {
		a.logger.Warn("Failed to create Docker client", "error", err)
	} else {
		// Test connection
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := dockerClient.Ping(ctx); err != nil {
			a.logger.Warn("Cannot connect to Docker daemon", "error", err)
		}
		cancel()

		// Create route sink
		routeSink := &appRouteSink{app: a}

		// Create discovery engine
		a.discoveryEngine = discovery.NewEngine(dockerClient, routeSink, a.logger)
	}

	return nil
}

func (a *App) start(ctx context.Context) {
	// Start health checker
	go a.healthChecker.Start(ctx)

	// Start discovery engine
	if a.discoveryEngine != nil {
		if err := a.discoveryEngine.Start(ctx); err != nil {
			a.logger.Error("Failed to start discovery engine", "error", err)
		}
	}

	// Start TLS renewal scheduler
	if a.tlsManager != nil {
		scheduler := tlspkg.NewRenewalScheduler(a.tlsManager, a.logger)
		scheduler.Start(ctx)
	}

	// Initialize proxy
	pxy := proxy.NewProxy(a.logger)

	// Initialize middleware builder (shared for cleanup)
	a.middlewareBuilder = router.NewRouteMiddlewareBuilder()

	// Initialize router with shared middleware builder
	httpRouter := router.NewRouterWithMiddleware(a.routeTable, pxy, a.logger, a.middlewareBuilder)

	// Build middleware chain
	coreHandler := a.buildMiddlewareChain(httpRouter)

	// HTTP handler with ACME challenge
	httpHandler := a.buildHTTPHandler(coreHandler)

	// Start HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", a.config.HTTPPort),
		Handler:      httpHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		a.logger.Info("HTTP server listening", "port", a.config.HTTPPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("HTTP server error", "error", err)
		}
	}()

	// Start HTTPS server
	if a.tlsManager != nil {
		httpsServer := &http.Server{
			Addr:         fmt.Sprintf(":%d", a.config.HTTPSPort),
			Handler:      coreHandler,
			TLSConfig:    a.tlsManager.GetTLSConfig(),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		}

		go func() {
			a.logger.Info("HTTPS server listening", "port", a.config.HTTPSPort)
			if err := httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				a.logger.Error("HTTPS server error", "error", err)
			}
		}()
	}

	// Start admin server
	if a.config.Admin {
		adminHandler := a.buildAdminHandler()
		adminAddr := fmt.Sprintf("%s:%d", a.config.AdminBind, a.config.AdminPort)

		go func() {
			a.logger.Info("Admin server listening", "addr", adminAddr)
			if err := http.ListenAndServe(adminAddr, adminHandler); err != nil {
				a.logger.Error("Admin server error", "error", err)
			}
		}()
	}
}

func (a *App) shutdown(ctx context.Context) {
	// Components will shut down via context cancellation
}

func (a *App) buildMiddlewareChain(handler http.Handler) http.Handler {
	chain := middleware.Chain(
		middleware.Recovery,
		middleware.RequestID,
	)

	if a.config.AccessLog {
		chain = middleware.Chain(chain, middleware.AccessLog)
	}

	chain = middleware.Chain(chain, middleware.SecurityHeaders)

	return chain(handler)
}

func (a *App) buildHTTPHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ACME challenge (highest priority)
		if a.challengeSolver.Matches(r.URL.Path) {
			a.challengeSolver.Handler().ServeHTTP(w, r)
			return
		}

		// HTTP to HTTPS redirect
		if a.config.DefaultTLS != "off" && r.TLS == nil {
			if r.Header.Get("X-Forwarded-Proto") != "https" {
				target := fmt.Sprintf("https://%s%s", r.Host, r.URL.Path)
				if r.URL.RawQuery != "" {
					target += "?" + r.URL.RawQuery
				}
				http.Redirect(w, r, target, http.StatusMovedPermanently)
				return
			}
		}

		handler.ServeHTTP(w, r)
	})
}

func (a *App) buildAdminHandler() http.Handler {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/v1/status", a.handleStatus)
	mux.HandleFunc("/api/v1/routes", a.handleRoutes)
	mux.HandleFunc("/api/v1/containers", a.handleContainers)
	mux.HandleFunc("/api/v1/certificates", a.handleCertificates)
	mux.HandleFunc("/api/v1/health", a.handleHealth)
	mux.HandleFunc("/api/v1/metrics", a.handleMetrics)
	mux.HandleFunc("/api/v1/config", a.handleConfig)

	// Dashboard
	dashboardRoot, _ := fs.Sub(dashboardFS, "dashboard")
	fileServer := http.FileServer(http.FS(dashboardRoot))
	mux.Handle("/static/", http.StripPrefix("/static/", fileServer))
	// Serve dashboard assets directly
	mux.HandleFunc("/style.css", a.serveDashboardAsset)
	mux.HandleFunc("/app.js", a.serveDashboardAsset)
	mux.HandleFunc("/", a.handleDashboard)

	// Apply auth if configured
	if a.config.AdminUser != "" {
		auth := admin.NewAuth(a.config.AdminUser, a.config.AdminPass)
		return auth.Middleware(mux)
	}

	return mux
}

// API Handlers

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(a.startTime)
	containers := 0
	certificates := 0
	if a.discoveryEngine != nil {
		containers = len(a.discoveryEngine.GetContainers())
	}
	if a.tlsManager != nil {
		certificates = len(a.tlsManager.ListCertificates())
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","version":"%s","routes":%d,"containers":%d,"certificates":%d,"uptime":"%s","http_port":%d,"https_port":%d}`,
		a.config.Version,
		a.routeTable.Count(),
		containers,
		certificates,
		uptime.Round(time.Second),
		a.config.HTTPPort,
		a.config.HTTPSPort,
	)
}

func (a *App) handleRoutes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	routes := a.routeTable.List()
	fmt.Fprintf(w, "[")
	for i, route := range routes {
		if i > 0 {
			fmt.Fprintf(w, ",")
		}
		backend := "-"
		if route.Backend != nil && len(route.Backend.Targets) > 0 {
			backend = route.Backend.Targets[0].Address
		}
		tlsStatus := route.TLS.Mode != ""
		fmt.Fprintf(w, `{"id":"%s","host":"%s","path_prefix":"%s","backend":"%s","tls":%v,"healthy":%v}`,
			route.ID[:12],
			route.Host,
			route.PathPrefix,
			backend,
			tlsStatus,
			route.Backend != nil && !route.Backend.AllUnhealthy(),
		)
	}
	fmt.Fprintf(w, "]")
}

func (a *App) handleContainers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if a.discoveryEngine == nil {
		fmt.Fprintf(w, "[]")
		return
	}
	containers := a.discoveryEngine.GetContainers()
	fmt.Fprintf(w, "[")
	for i, c := range containers {
		if i > 0 {
			fmt.Fprintf(w, ",")
		}
		status := "running"
		if !c.Healthy {
			status = "unhealthy"
		}
		// Count dr.* labels
		drLabelCount := 0
		for label := range c.Labels {
			if strings.HasPrefix(label, "dr.") {
				drLabelCount++
			}
		}
		fmt.Fprintf(w, `{"id":"%s","name":"%s","image":"%s","host":"%s","address":"%s","running":true,"status":"%s","healthy":%v,"labels":%d}`,
			c.ID[:12],
			c.Name,
			c.Image,
			c.Config.Host,
			c.Address,
			status,
			c.Healthy,
			drLabelCount,
		)
	}
	fmt.Fprintf(w, "]")
}

func (a *App) handleCertificates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if a.tlsManager == nil {
		fmt.Fprintf(w, "[]")
		return
	}
	domains := a.tlsManager.ListCertificates()
	fmt.Fprintf(w, "[")
	for i, domain := range domains {
		if i > 0 {
			fmt.Fprintf(w, ",")
		}
		fmt.Fprintf(w, `{"domain":"%s"}`, domain)
	}
	fmt.Fprintf(w, "]")
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"healthy"}`))
}

func (a *App) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	a.metrics.PrometheusFormat(w)
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"http_port":%d,"https_port":%d,"admin":%v,"acme_email":"%s","log_level":"%s"}`,
		a.config.HTTPPort,
		a.config.HTTPSPort,
		a.config.Admin,
		a.config.ACMEEmail,
		a.config.LogLevel,
	)
}

func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Serve embedded index.html
	data, err := dashboardFS.ReadFile("dashboard/index.html")
	if err != nil {
		http.Error(w, "Dashboard not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func (a *App) serveDashboardAsset(w http.ResponseWriter, r *http.Request) {
	// Get filename from path
	filename := r.URL.Path[1:] // Remove leading slash

	// Serve embedded file
	data, err := dashboardFS.ReadFile("dashboard/" + filename)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Set content type
	switch {
	case strings.HasSuffix(filename, ".css"):
		w.Header().Set("Content-Type", "text/css")
	case strings.HasSuffix(filename, ".js"):
		w.Header().Set("Content-Type", "application/javascript")
	}

	w.Write(data)
}

// Route sink adapter

type appRouteSink struct {
	app *App
}

func (s *appRouteSink) AddRoute(info *discovery.ContainerInfo) {
	pool := router.NewBackendPool(router.RoundRobin)
	pool.Add(&router.BackendTarget{
		Address:     info.Address,
		ContainerID: info.ID,
		Healthy:     info.Healthy,
	})

	route := &router.Route{
		ID:            info.ID,
		Host:          info.Config.Host,
		PathPrefix:    info.Config.Path,
		Backend:       pool,
		ContainerID:   info.ID,
		ContainerName: info.Name,
		CreatedAt:     time.Now(),
	}

	// Apply middleware configuration from labels
	route.MiddlewareConfig = router.MiddlewareConfig{
		RateLimit: router.RateLimitConfig{
			Enabled: info.Config.RateLimit.Enabled,
			Count:   info.Config.RateLimit.Count,
			Window:  info.Config.RateLimit.Window,
			ByKey:   info.Config.RateLimit.ByKey,
		},
		CORS: router.CORSConfig{
			Enabled: info.Config.CORS.Enabled,
			Origins: info.Config.CORS.Origins,
			Methods: info.Config.CORS.Methods,
			Headers: info.Config.CORS.Headers,
		},
		Compress:    info.Config.Compress,
		StripPrefix: info.Config.StripPrefix,
		AddPrefix:   info.Config.AddPrefix,
		MaxBody:     info.Config.MaxBody,
		CircuitBreaker: router.CircuitBreakerConfig{
			Enabled:  info.Config.CircuitBreaker.Enabled,
			Failures: info.Config.CircuitBreaker.Failures,
			Window:   info.Config.CircuitBreaker.Window,
		},
	}

	// Copy basic auth users
	for _, u := range info.Config.BasicAuthUsers {
		route.MiddlewareConfig.BasicAuthUsers = append(route.MiddlewareConfig.BasicAuthUsers,
			router.BasicAuthUser{Username: u.Username, Hash: u.Hash})
	}

	// Copy IP whitelists/blacklists
	route.MiddlewareConfig.IPWhitelist = info.Config.IPWhitelist
	route.MiddlewareConfig.IPBlacklist = info.Config.IPBlacklist

	if info.Config.TLS != "off" {
		route.TLS = router.TLSConfig{
			Mode:    info.Config.TLS,
			Domains: info.Config.TLSDomains,
		}

		// Trigger certificate provisioning
		if s.app.tlsManager != nil && info.Config.TLS == "auto" {
			go func() {
				if err := s.app.tlsManager.EnsureCertificate(info.Config.Host); err != nil {
					s.app.logger.Error("Failed to provision certificate",
						"domain", info.Config.Host,
						"error", err,
					)
				}
			}()
		}
	}

	s.app.routeTable.Add(route)
	s.app.logger.Info("Route added",
		"container", info.Name,
		"host", info.Config.Host,
		"address", info.Address,
	)
}

func (s *appRouteSink) RemoveRoute(containerID string) {
	s.app.routeTable.RemoveByContainer(containerID)
	s.app.logger.Info("Route removed", "container_id", containerID[:12])
}

func parseLogLevel(level string) log.Level {
	switch level {
	case "debug":
		return log.LevelDebug
	case "warn":
		return log.LevelWarn
	case "error":
		return log.LevelError
	default:
		return log.LevelInfo
	}
}
