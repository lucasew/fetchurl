package app

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/lucasew/fetchurl/internal/db"
	"github.com/lucasew/fetchurl/internal/eviction"
	_ "github.com/lucasew/fetchurl/internal/eviction/lru"
	"github.com/lucasew/fetchurl/internal/eviction/policy"
	"github.com/lucasew/fetchurl/internal/eviction/policy/maxsize"
	"github.com/lucasew/fetchurl/internal/eviction/policy/minfree"
	"github.com/lucasew/fetchurl/internal/fetcher"
	"github.com/lucasew/fetchurl/internal/handler"
	"github.com/lucasew/fetchurl/internal/proxy"
	"github.com/lucasew/fetchurl/internal/repository"
)

type Config struct {
	Port             int
	CacheDir         string
	MaxCacheSize     int64
	MinFreeSpace     int64
	EvictionInterval time.Duration
	EvictionStrategy string
	Upstreams        []string
	CaCert           string
	CaKey            string
}

// loadCAContent resolves the CA content from path, hex, or raw string.
func loadCAContent(input string) ([]byte, error) {
	if input == "" {
		return nil, nil
	}

	// 1. Try file path (naive check: if file exists)
	// Note: If input is a path that doesn't exist but is meant to be content, this might fail or be skipped.
	// However, usually paths are short and content is long (PEM).
	// Let's rely on standard practice: if it looks like PEM (contains -----BEGIN), treat as content.
	// If it is hex (no PEM headers, only hex chars), treat as hex.
	// Else try file.

	// Check for PEM
	if regexp.MustCompile(`-----BEGIN`).MatchString(input) {
		return []byte(input), nil
	}

	// Check for Hex
	// Hex string usually doesn't contain spaces/newlines if passed via cli/env correctly,
	// but might have them if copy-pasted. Let's assume strict hex for now or sanitized.
	// A simple check: if it decodes successfully as hex and is long enough.
	// But "deadbeef" is valid hex and also a valid file path potentially.
	// Let's prioritize file if it exists.

	// Try Hex decode
	// Prioritize hex if it looks like hex (no file path separators and valid hex)
	// This avoids ambiguity if a file named "deadbeef" exists but we meant content,
	// but generally file paths contain / or .
	if !regexp.MustCompile(`[/\\]`).MatchString(input) {
		if bytes, err := hex.DecodeString(input); err == nil && len(bytes) > 0 {
			return bytes, nil
		}
	}

	// Try reading file
	if _, err := os.Stat(input); err == nil {
		return os.ReadFile(input)
	}

	// Fallback: treat as raw bytes (though likely invalid if not PEM)
	return []byte(input), nil
}

func NewServer(cfg Config) (*http.Server, func(), error) {
	// Setup Eviction Manager
	strat, err := eviction.GetStrategy(cfg.EvictionStrategy)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize eviction strategy: %w", err)
	}

	// Setup Policies
	var policies []policy.Policy

	if cfg.MaxCacheSize > 0 {
		slog.Info("Adding MaxCacheSize policy", "max_size", cfg.MaxCacheSize)
		policies = append(policies, &maxsize.Policy{MaxBytes: cfg.MaxCacheSize})
	}

	if cfg.MinFreeSpace > 0 {
		slog.Info("Adding MinFreeSpace policy", "min_free", cfg.MinFreeSpace)
		policies = append(policies, &minfree.Policy{
			Path:         cfg.CacheDir,
			MinFreeBytes: cfg.MinFreeSpace,
		})
	}

	if len(policies) == 0 {
		slog.Info("No eviction policies configured (unlimited cache)")
	}

	mgr := eviction.NewManager(cfg.CacheDir, policies, cfg.EvictionInterval, strat)

	if err := mgr.LoadInitialState(); err != nil {
		slog.Warn("Failed to load initial cache state", "error", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Start eviction manager
	go mgr.Start(ctx)

	// Setup DB
	dbPath := filepath.Join(cfg.CacheDir, "links.db")
	database, err := db.Open(dbPath)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("failed to open database at %s: %w", dbPath, err)
	}

	localRepo := repository.NewLocalRepository(cfg.CacheDir, mgr)
	var upstreamRepos []repository.Repository
	for _, u := range cfg.Upstreams {
		upstreamRepos = append(upstreamRepos, repository.NewUpstreamRepository(u))
	}

	fetchService := fetcher.NewService(upstreamRepos)
	casHandler := handler.NewCASHandler(localRepo, fetchService)

	// Fallback Mux for explicit /fetch/ routes
	fallbackMux := http.NewServeMux()
	fallbackMux.Handle("/fetch/", casHandler)

	// Setup Proxy Rules
	// Default rule: matches sha256 hashes in URL path
	sha256Rule := proxy.NewRegexRule(
		regexp.MustCompile(`sha256/(?P<hash>[a-f0-9]{64})`),
		"sha256",
	)

	// DB Rule
	dbRule := proxy.NewDBRule(database, "sha256")
	dbRuleSha1 := proxy.NewDBRule(database, "sha1")

	rules := []proxy.Rule{sha256Rule, dbRule, dbRuleSha1}

	var caCert *tls.Certificate
	var errCert error

	if cfg.CaCert != "" && cfg.CaKey != "" {
		slog.Info("Loading CA certificate")
		certBytes, err := loadCAContent(cfg.CaCert)
		if err != nil {
			errCert = fmt.Errorf("failed to load CA cert: %w", err)
		}
		keyBytes, err := loadCAContent(cfg.CaKey)
		if err != nil {
			errCert = fmt.Errorf("failed to load CA key: %w", err)
		}

		if errCert == nil {
			cert, err := tls.X509KeyPair(certBytes, keyBytes)
			if err != nil {
				errCert = fmt.Errorf("failed to parse CA keypair: %w", err)
			} else {
				caCert = &cert
			}
		}
	}

	if errCert != nil {
		cancel()
		return nil, nil, errCert
	}

	// Initialize Proxy Server with fallback Mux
	proxyServer := proxy.NewServer(localRepo, fetchService, rules, fallbackMux, caCert)

	// Add NPM Interceptor
	proxyServer.Proxy.OnResponse().Do(proxy.NewNpmResponseHandler(database.Queries))

	addr := fmt.Sprintf(":%d", cfg.Port)
	slog.Info("Starting server (Proxy + CAS)", "addr", addr, "cache_dir", cfg.CacheDir, "db_path", dbPath)

	server := &http.Server{
		Addr:    addr,
		Handler: proxyServer.Proxy,
	}

	cleanup := func() {
		database.Close()
		cancel()
	}

	return server, cleanup, nil
}
