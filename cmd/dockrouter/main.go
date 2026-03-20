// Package main is the entry point for DockRouter
package main

import (
	"context"
	"embed"
	"encoding/json"
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

// healthCheckURL is the URL for health checks (can be overridden in tests)
var healthCheckURL = "http://localhost:9090/api/v1/health"

// Embed dashboard files
//
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

	// TLS renewal
	renewalScheduler *tlspkg.RenewalScheduler

	// HTTP servers for graceful shutdown
	httpServer  *http.Server
	httpsServer *http.Server
	adminServer *http.Server
}

func main() {
	// Handle healthcheck command (for Docker HEALTHCHECK)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		doHealthCheck()
		return
	}

	// Handle version command
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "-v" || os.Args[1] == "--version") {
		printVersion()
		return
	}

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

	// Cancel context first to stop discovery engine goroutines
	cancel()

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	app.shutdown(shutdownCtx)

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
		a.renewalScheduler = tlspkg.NewRenewalScheduler(a.tlsManager, a.logger)
		a.renewalScheduler.Start(ctx)
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
	a.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", a.config.HTTPPort),
		Handler:      httpHandler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		a.logger.Info("HTTP server listening", "port", a.config.HTTPPort)
		if err := a.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.logger.Error("HTTP server error", "error", err)
		}
	}()

	// Start HTTPS server
	if a.tlsManager != nil {
		a.httpsServer = &http.Server{
			Addr:         fmt.Sprintf(":%d", a.config.HTTPSPort),
			Handler:      coreHandler,
			TLSConfig:    a.tlsManager.GetTLSConfig(),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		}

		go func() {
			a.logger.Info("HTTPS server listening", "port", a.config.HTTPSPort)
			if err := a.httpsServer.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				a.logger.Error("HTTPS server error", "error", err)
			}
		}()
	}

	// Start admin server
	if a.config.Admin {
		adminHandler := a.buildAdminHandler()
		adminAddr := fmt.Sprintf("%s:%d", a.config.AdminBind, a.config.AdminPort)

		a.adminServer = &http.Server{
			Addr:         adminAddr,
			Handler:      adminHandler,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		go func() {
			a.logger.Info("Admin server listening", "addr", adminAddr)
			if err := a.adminServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				a.logger.Error("Admin server error", "error", err)
			}
		}()
	}
}

func (a *App) shutdown(ctx context.Context) {
	a.logger.Info("Shutting down servers...")

	// Shutdown HTTP server
	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil {
			a.logger.Error("HTTP server shutdown error", "error", err)
		} else {
			a.logger.Info("HTTP server stopped")
		}
	}

	// Shutdown HTTPS server
	if a.httpsServer != nil {
		if err := a.httpsServer.Shutdown(ctx); err != nil {
			a.logger.Error("HTTPS server shutdown error", "error", err)
		} else {
			a.logger.Info("HTTPS server stopped")
		}
	}

	// Shutdown admin server
	if a.adminServer != nil {
		if err := a.adminServer.Shutdown(ctx); err != nil {
			a.logger.Error("Admin server shutdown error", "error", err)
		} else {
			a.logger.Info("Admin server stopped")
		}
	}

	a.logger.Info("All servers stopped")
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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"version":      a.config.Version,
		"routes":       a.routeTable.Count(),
		"containers":   containers,
		"certificates": certificates,
		"uptime":       uptime.Round(time.Second).String(),
		"http_port":    a.config.HTTPPort,
		"https_port":   a.config.HTTPSPort,
	})
}

func (a *App) handleRoutes(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	routes := a.routeTable.List()
	type routeEntry struct {
		ID         string `json:"id"`
		Host       string `json:"host"`
		PathPrefix string `json:"path_prefix"`
		Backend    string `json:"backend"`
		TLS        bool   `json:"tls"`
		Healthy    bool   `json:"healthy"`
	}
	entries := make([]routeEntry, 0, len(routes))
	for _, route := range routes {
		backend := "-"
		if route.Backend != nil && len(route.Backend.Targets) > 0 {
			backend = route.Backend.Targets[0].Address
		}
		entries = append(entries, routeEntry{
			ID:         truncateID(route.ID),
			Host:       route.Host,
			PathPrefix: route.PathPrefix,
			Backend:    backend,
			TLS:        route.TLS.Mode != "",
			Healthy:    route.Backend != nil && !route.Backend.AllUnhealthy(),
		})
	}
	json.NewEncoder(w).Encode(entries)
}

func (a *App) handleContainers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if a.discoveryEngine == nil {
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}
	containers := a.discoveryEngine.GetContainers()
	type containerEntry struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Image   string `json:"image"`
		Host    string `json:"host"`
		Address string `json:"address"`
		Running bool   `json:"running"`
		Status  string `json:"status"`
		Healthy bool   `json:"healthy"`
		Labels  int    `json:"labels"`
	}
	entries := make([]containerEntry, 0, len(containers))
	for _, c := range containers {
		status := "running"
		if !c.Healthy {
			status = "unhealthy"
		}
		drLabelCount := 0
		for label := range c.Labels {
			if strings.HasPrefix(label, "dr.") {
				drLabelCount++
			}
		}
		host := ""
		if c.Config != nil {
			host = c.Config.Host
		}
		entries = append(entries, containerEntry{
			ID:      truncateID(c.ID),
			Name:    c.Name,
			Image:   c.Image,
			Host:    host,
			Address: c.Address,
			Running: true,
			Status:  status,
			Healthy: c.Healthy,
			Labels:  drLabelCount,
		})
	}
	json.NewEncoder(w).Encode(entries)
}

func (a *App) handleCertificates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if a.tlsManager == nil {
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}
	domains := a.tlsManager.ListCertificates()
	type certEntry struct {
		Domain string `json:"domain"`
	}
	entries := make([]certEntry, 0, len(domains))
	for _, domain := range domains {
		entries = append(entries, certEntry{Domain: domain})
	}
	json.NewEncoder(w).Encode(entries)
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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"http_port":  a.config.HTTPPort,
		"https_port": a.config.HTTPSPort,
		"admin":      a.config.Admin,
		"acme_email": a.config.ACMEEmail,
		"log_level":  a.config.LogLevel,
	})
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
	pool := router.NewBackendPool(router.ParseLoadBalanceStrategy(info.Config.LoadBalance))
	pool.Add(&router.BackendTarget{
		Address:     info.Address,
		ContainerID: info.ID,
		Weight:      info.Config.Weight,
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
		Retry: info.Config.Retry,
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
	s.app.logger.Info("Route removed", "container_id", truncateID(containerID))
}

// truncateID safely truncates an ID to 12 characters for display
func truncateID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
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

// doHealthCheck performs a health check for Docker HEALTHCHECK
func doHealthCheck() {
	if err := performHealthCheck(); err != nil {
		fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("healthy")
	os.Exit(0)
}

// performHealthCheck performs the actual health check and returns an error if it fails
// This is extracted for testability
func performHealthCheck() error {
	// Check admin endpoint health
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(healthCheckURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// printVersion prints version information
func printVersion() {
	fmt.Printf("DockRouter %s\n", version)
	fmt.Printf("  Built: %s\n", buildTime)
	fmt.Println()
	fmt.Println("Zero-dependency Docker-native ingress router")
	fmt.Println("https://github.com/DockRouter/dockrouter")
}
