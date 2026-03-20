// Package tls handles TLS certificate management
package tls

import (
	"context"
	"sync"
	"time"
)

// RenewalScheduler handles automatic certificate renewal
type RenewalScheduler struct {
	manager  *Manager
	interval time.Duration
	logger   Logger
	wg       sync.WaitGroup
	cancel   context.CancelFunc
}

// Logger interface for TLS package
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// NewRenewalScheduler creates a new renewal scheduler
func NewRenewalScheduler(manager *Manager, logger Logger) *RenewalScheduler {
	return &RenewalScheduler{
		manager:  manager,
		interval: 24 * time.Hour,
		logger:   logger,
	}
}

// Start begins the renewal check loop. It is not safe to call Start more than once.
func (s *RenewalScheduler) Start(ctx context.Context) {
	if s.cancel != nil {
		return // already started
	}
	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go s.run(ctx)
}

func (s *RenewalScheduler) run(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Initial check
	s.checkRenewals()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkRenewals()
		}
	}
}

func (s *RenewalScheduler) checkRenewals() {
	domains, err := s.manager.store.List()
	if err != nil {
		s.logger.Error("Failed to list certificates for renewal check", "error", err)
		return
	}

	for _, domain := range domains {
		certPEM, _, err := s.manager.store.LoadPEM(domain)
		if err != nil {
			s.logger.Warn("Failed to load certificate for renewal check",
				"domain", domain,
				"error", err,
			)
			continue
		}

		if ShouldRenew(certPEM) {
			s.logger.Info("Certificate needs renewal",
				"domain", domain,
			)

			if err := s.manager.Renew(domain); err != nil {
				s.logger.Error("Certificate renewal failed",
					"domain", domain,
					"error", err,
				)
			} else {
				s.logger.Info("Certificate renewed successfully",
					"domain", domain,
				)
			}
		}
	}
}

// Stop stops the scheduler and waits for it to finish
func (s *RenewalScheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}
