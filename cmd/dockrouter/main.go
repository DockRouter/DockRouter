// Package main is the entry point for DockRouter
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
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

// Build-time variables (set via ldflags).
// The names must match the -X targets in the Makefile, Dockerfile and
// .goreleaser.yml; a mismatched name is silently ignored by the linker.
var (
	version   = "dev"
	buildTime = "unknown"
	commit    = "unknown"
)

// healthCheckURL is the URL for health checks (can be overridden in tests).
// It targets the unauthenticated /health probe rather than the admin API, so
// the Docker HEALTHCHECK still works when DR_ADMIN_USER is set.
var healthCheckURL = "http://localhost:9090/health"

// healthCheckClient is a reusable HTTP client for health checks
var healthCheckClient = &http.Client{
	Timeout: 2 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        1,
		MaxIdleConnsPerHost: 1,
		IdleConnTimeout:     30 * time.Second,
		DisableKeepAlives:   false,
	},
}

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

	// Initialize health checker and feed its results back into the route table
	// so a backend that fails and later recovers returns to rotation.
	a.healthChecker = health.NewChecker(10*time.Second, 5*time.Second)
	a.healthChecker.OnStateChange(func(target string, state health.HealthState) {
		healthy := state == health.StateHealthy || state == health.StateDegraded
		a.routeTable.SetBackendHealth(target, healthy)
		a.logger.Info("Backend health changed",
			"target", target,
			"state", state.String(),
			"in_rotation", healthy,
		)
	})

	// Initialize challenge solver
	a.challengeSolver = tlspkg.NewChallengeSolver()

	// Initialize TLS components whenever TLS is enabled at all. The ACME client
	// is optional: without an account email we can still serve certificates
	// that were provisioned earlier or mounted manually, which is what
	// dr.tls=manual relies on. An explicit ACME email enables TLS even when the
	// global default is "off", because routes can opt in with dr.tls=auto.
	if a.config.DefaultTLS != "off" || a.config.ACMEEmail != "" {
		tlsStore := tlspkg.NewStore(a.config.DataDir)

		var acmeClient *tlspkg.ACMEClient
		if a.config.ACMEEmail != "" {
			acmeClient = tlspkg.NewACMEClient(a.config.GetACMEDirectoryURL(), a.config.ACMEEmail)
			if err := acmeClient.Initialize(); err != nil {
				a.logger.Warn("Failed to initialize ACME client", "error", err)
			}
		} else {
			a.logger.Warn("No ACME email configured; automatic certificate issuance is disabled",
				"hint", "set DR_ACME_EMAIL to enable Let's Encrypt")
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
	// Initialize middleware builder before launching any goroutines
	a.middlewareBuilder = router.NewRouteMiddlewareBuilder()
	if len(a.config.TrustedIPs) > 0 {
		a.middlewareBuilder.SetTrustedProxies(a.config.TrustedIPs)
		a.logger.Info("Trusted proxies configured", "cidrs", a.config.TrustedIPs)
	}

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

	// Initialize router with shared middleware builder
	httpRouter := router.NewRouterWithMiddleware(a.routeTable, pxy, a.logger, a.middlewareBuilder)

	// Build middleware chain
	coreHandler := a.buildMiddlewareChain(httpRouter)

	// HTTP handler with ACME challenge
	httpHandler := a.buildHTTPHandler(coreHandler)

	// Start HTTP server
	a.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", a.config.HTTPPort),
		Handler: httpHandler,
		// No ReadTimeout/WriteTimeout: they are wall-clock deadlines for the
		// whole exchange and would sever WebSocket and SSE connections. The
		// header deadline still bounds slow-loris style attacks.
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
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
			Addr:      fmt.Sprintf(":%d", a.config.HTTPSPort),
			Handler:   coreHandler,
			TLSConfig: a.tlsManager.GetTLSConfig(),
			// No ReadTimeout/WriteTimeout: they are wall-clock deadlines for the
			// whole exchange and would sever WebSocket and SSE connections. The
			// header deadline still bounds slow-loris style attacks.
			ReadHeaderTimeout: 30 * time.Second,
			IdleTimeout:       120 * time.Second,
			MaxHeaderBytes:    1 << 20, // 1MB
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
			Addr:           adminAddr,
			Handler:        adminHandler,
			ReadTimeout:    10 * time.Second,
			WriteTimeout:   10 * time.Second,
			MaxHeaderBytes: 1 << 20, // 1MB
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

	// Without this the collector stays empty and /metrics reports nothing.
	if a.metrics != nil {
		chain = middleware.Chain(chain, middleware.Metrics(a.metrics))
	}

	if a.config.AccessLog {
		chain = middleware.Chain(chain, middleware.AccessLog)
	}

	// 30s request timeout
	chain = middleware.Chain(chain, middleware.Timeout(30*time.Second))

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
				host, _, err := net.SplitHostPort(r.Host)
				if err != nil {
					host = r.Host
				}

				var route *router.Route
				if a.routeTable != nil {
					route = a.routeTable.Match(host, r.URL.Path)
				}
				if route == nil {
					http.Error(w, "Bad Request", http.StatusBadRequest)
					return
				}

				// Decided per route rather than globally: a route with
				// dr.tls=off is served over plain HTTP instead of being
				// redirected to a port that holds no certificate for it.
				if a.shouldRedirectToHTTPS(route) {
					target := fmt.Sprintf("https://%s%s", r.Host, r.URL.Path)
					if r.URL.RawQuery != "" {
						target += "?" + r.URL.RawQuery
					}
					http.Redirect(w, r, target, http.StatusMovedPermanently)
					return
				}
			}
		}

		handler.ServeHTTP(w, r)
	})
}

// shouldRedirectToHTTPS reports whether a matched route should be redirected
// from HTTP to HTTPS. The route's own TLS mode wins; only routes that never
// declared one fall back to the global default.
func (a *App) shouldRedirectToHTTPS(route *router.Route) bool {
	mode := route.TLS.Mode
	if mode == "" {
		mode = a.config.DefaultTLS
	}
	return mode != "off"
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

	// Top-level monitoring endpoints. Probes and Prometheus scrapers expect
	// these paths, not the versioned API ones.
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/ready", a.handleReady)
	mux.HandleFunc("/metrics", a.handleMetrics)

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
		protected := auth.Middleware(mux)

		// The liveness and readiness probes stay reachable without credentials:
		// the Docker HEALTHCHECK and orchestrator probes do not authenticate,
		// and a 401 would mark a healthy container as failed. They expose only
		// liveness state. The admin API, including /api/v1/health, stays behind
		// authentication.
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/health", "/ready":
				mux.ServeHTTP(w, r)
			default:
				protected.ServeHTTP(w, r)
			}
		})
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

// handleReady reports whether DockRouter can actually serve traffic. It is
// distinct from /health: the process can be alive while Docker discovery is
// still down, in which case there are no routes to serve.
func (a *App) handleReady(w http.ResponseWriter, r *http.Request) {
	ready := true
	reason := "ok"

	if a.discoveryEngine != nil && !a.discoveryEngine.IsRunning() {
		ready = false
		reason = "docker discovery not running"
	}

	routes := 0
	if a.routeTable != nil {
		routes = a.routeTable.Count()
	}

	w.Header().Set("Content-Type", "application/json")
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":  ready,
		"reason": reason,
		"routes": routes,
	})
}

func (a *App) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	if a.metrics == nil {
		return
	}
	a.sampleStateMetrics()
	a.metrics.PrometheusFormat(w)
}

// sampleStateMetrics refreshes the metrics that describe current state rather
// than accumulated events. These are cheaper to sample at scrape time than to
// keep continuously in sync.
func (a *App) sampleStateMetrics() {
	if a.routeTable != nil {
		a.metrics.SetGauge("routes_total", float64(a.routeTable.Count()))
	}

	containers := 0
	if a.discoveryEngine != nil {
		containers = len(a.discoveryEngine.GetContainers())
	}
	a.metrics.SetGauge("containers_total", float64(containers))

	certificates := 0
	if a.tlsManager != nil {
		certificates = len(a.tlsManager.ListCertificates())
	}
	a.metrics.SetGauge("certificates_total", float64(certificates))

	if a.routeTable == nil {
		return
	}

	var requests, errors, active int64
	for _, route := range a.routeTable.List() {
		if route.Backend == nil {
			continue
		}
		for _, target := range route.Backend.Snapshot() {
			rq, fl, ac := target.Stats()
			requests += rq
			errors += fl
			active += ac
		}
	}
	a.metrics.SetGauge("backend_requests_total", float64(requests))
	a.metrics.SetGauge("backend_errors_total", float64(errors))
	a.metrics.SetGauge("active_connections", float64(active))
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
	target := &router.BackendTarget{
		Address:     info.Address,
		ContainerID: info.ID,
		Weight:      info.Config.Weight,
		Healthy:     info.Healthy,
	}

	route := &router.Route{
		ID:            router.RouteKey(info.Config.Host, info.Config.Path),
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
			Mode:     info.Config.TLS,
			Domains:  info.Config.TLSDomains,
			CertFile: info.Config.TLSCertFile,
			KeyFile:  info.Config.TLSKeyFile,
		}

		if s.app.tlsManager != nil {
			switch info.Config.TLS {
			case "auto":
				// Trigger certificate provisioning, covering any extra SAN
				// domains declared with dr.tls.domains.
				sans := append([]string(nil), info.Config.TLSDomains...)
				go func() {
					if err := s.app.tlsManager.EnsureCertificate(info.Config.Host, sans...); err != nil {
						s.app.logger.Error("Failed to provision certificate",
							"domain", info.Config.Host,
							"sans", sans,
							"error", err,
						)
					}
				}()

			case "manual":
				// Load the operator-supplied certificate for this host and any
				// additional SAN domains it is meant to serve.
				domains := append([]string{info.Config.Host}, info.Config.TLSDomains...)
				for _, domain := range domains {
					if err := s.app.tlsManager.LoadManualCertificate(
						domain, info.Config.TLSCertFile, info.Config.TLSKeyFile,
					); err != nil {
						s.app.logger.Error("Failed to load manual certificate",
							"domain", domain,
							"error", err,
						)
					}
				}
			}
		}
	}

	// Upsert merges replicas that share a host and path into one backend pool,
	// which is what lets the load-balancing strategies do their job.
	s.app.routeTable.Upsert(route, target)

	// Register the backend for active health checking so that a backend which
	// fails and later recovers is returned to rotation.
	if s.app.healthChecker != nil {
		s.app.healthChecker.Register(info.Address, health.HealthCheck{
			Target:    info.Address,
			Path:      info.Config.HealthCheck.Path,
			Interval:  info.Config.HealthCheck.Interval,
			Timeout:   info.Config.HealthCheck.Timeout,
			Threshold: info.Config.HealthCheck.Threshold,
			Recovery:  info.Config.HealthCheck.Recovery,
		})
	}

	s.app.logger.Info("Route added",
		"container", info.Name,
		"host", info.Config.Host,
		"address", info.Address,
	)
}

func (s *appRouteSink) RemoveRoute(containerID string) {
	// Drop only this container's backend; sibling replicas keep serving.
	s.app.routeTable.RemoveByContainer(containerID)

	if s.app.healthChecker != nil {
		for _, addr := range s.app.staleHealthTargets() {
			s.app.healthChecker.Unregister(addr)
		}
	}

	s.app.logger.Info("Route removed", "container_id", truncateID(containerID))
}

// staleHealthTargets returns health-check targets that no route serves anymore.
func (a *App) staleHealthTargets() []string {
	live := make(map[string]bool)
	for _, addr := range a.routeTable.BackendAddresses() {
		live[addr] = true
	}

	stale := make([]string, 0)
	for _, addr := range a.healthChecker.Targets() {
		if !live[addr] {
			stale = append(stale, addr)
		}
	}
	return stale
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

// healthCheckEndpoint resolves the admin health URL. It honours the same
// environment variables the server reads, so a non-default admin port still
// produces a working `dockrouter healthcheck`.
func healthCheckEndpoint() string {
	port := os.Getenv("DR_ADMIN_PORT")
	if port == "" {
		return healthCheckURL
	}

	host := os.Getenv("DR_ADMIN_BIND")
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port) + "/health"
}

// performHealthCheck performs the actual health check and returns an error if it fails
// This is extracted for testability
func performHealthCheck() error {
	// Check admin endpoint health
	resp, err := healthCheckClient.Get(healthCheckEndpoint())
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
	fmt.Printf("  Built:  %s\n", buildTime)
	fmt.Printf("  Commit: %s\n", commit)
	fmt.Println()
	fmt.Println("Zero-dependency Docker-native ingress router")
	fmt.Println("https://github.com/DockRouter/dockrouter")
}
