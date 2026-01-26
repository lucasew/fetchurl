package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/lucasew/fetchurl/internal/eviction"
	_ "github.com/lucasew/fetchurl/internal/eviction/lru"
	"github.com/lucasew/fetchurl/internal/eviction/policy"
	"github.com/lucasew/fetchurl/internal/eviction/policy/maxsize"
	"github.com/lucasew/fetchurl/internal/eviction/policy/minfree"
	"github.com/lucasew/fetchurl/internal/fetcher"
	"github.com/lucasew/fetchurl/internal/proxy"
	"github.com/lucasew/fetchurl/internal/repository"
)

// NewProxyServer creates a new HTTP Proxy server with CAS capabilities.
func NewProxyServer(cfg Config) (*http.Server, func(), error) {
	// Setup Eviction Manager
	strat, err := eviction.GetStrategy(cfg.EvictionStrategy)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize eviction strategy: %w", err)
	}

	var policies []policy.Policy
	if cfg.MaxCacheSize > 0 {
		policies = append(policies, &maxsize.Policy{MaxBytes: cfg.MaxCacheSize})
	}
	if cfg.MinFreeSpace > 0 {
		policies = append(policies, &minfree.Policy{Path: cfg.CacheDir, MinFreeBytes: cfg.MinFreeSpace})
	}

	mgr := eviction.NewManager(cfg.CacheDir, policies, cfg.EvictionInterval, strat)
	if err := mgr.LoadInitialState(); err != nil {
		slog.Warn("Failed to load initial cache state", "error", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go mgr.Start(ctx)

	localRepo := repository.NewLocalRepository(cfg.CacheDir, mgr)
	var upstreamRepos []repository.Repository
	for _, u := range cfg.Upstreams {
		upstreamRepos = append(upstreamRepos, repository.NewUpstreamRepository(u))
	}
	fetchService := fetcher.NewService(upstreamRepos)

	// Setup Rules
	// TODO: Make this configurable via config file or flags
	// Example rule: matches ".../sha256/1234..."
	sha256Rule := &proxy.RegexRule{
		Regex: regexp.MustCompile(`sha256/(?P<hash>[a-f0-9]{64})`),
		Algo:  "sha256",
	}
	rules := []proxy.Rule{sha256Rule}

	pServer := proxy.NewServer(localRepo, fetchService, rules)

	addr := fmt.Sprintf(":%d", cfg.Port)
	slog.Info("Starting proxy server", "addr", addr, "cache_dir", cfg.CacheDir)

	server := &http.Server{
		Addr:    addr,
		Handler: pServer.Proxy,
	}

	cleanup := func() {
		cancel()
	}

	return server, cleanup, nil
}
