package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/lucasew/fetchurl/internal/eviction"
	_ "github.com/lucasew/fetchurl/internal/eviction/lru"
	"github.com/lucasew/fetchurl/internal/eviction/policy"
	"github.com/lucasew/fetchurl/internal/eviction/policy/maxsize"
	"github.com/lucasew/fetchurl/internal/eviction/policy/minfree"
	"github.com/lucasew/fetchurl/internal/handler"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Starts the HTTP server",
	Run: func(cmd *cobra.Command, args []string) {
		port := viper.GetInt("port")
		cacheDir := viper.GetString("cache-dir")
		maxCacheSize := viper.GetInt64("max-cache-size")
		minFreeSpace := viper.GetInt64("min-free-space")
		evictionInterval := viper.GetDuration("eviction-interval")
		evictionStrategy := viper.GetString("eviction-strategy")

		// Setup Eviction Manager
		strat, err := eviction.GetStrategy(evictionStrategy)
		if err != nil {
			slog.Error("Failed to initialize eviction strategy", "error", err)
			os.Exit(1)
		}

		// Setup Policies
		var policies []policy.Policy

		if maxCacheSize > 0 {
			slog.Info("Adding MaxCacheSize policy", "max_size", maxCacheSize)
			policies = append(policies, &maxsize.Policy{MaxBytes: maxCacheSize})
		}

		if minFreeSpace > 0 {
			slog.Info("Adding MinFreeSpace policy", "min_free", minFreeSpace)
			policies = append(policies, &minfree.Policy{
				Path:         cacheDir,
				MinFreeBytes: minFreeSpace,
			})
		}

		if len(policies) == 0 {
			// Default to 1GB max size if nothing configured?
			// Or should we trust default flag values?
			// Cobra flags have defaults, so maxCacheSize should be 1GB by default.
			// However, if user explicitly sets 0 to disable, we might have no policies.
			// That's fine, it means "unlimited".
			slog.Info("No eviction policies configured (unlimited cache)")
		}

		mgr := eviction.NewManager(cacheDir, policies, evictionInterval, strat)

		if err := mgr.LoadInitialState(); err != nil {
			slog.Warn("Failed to load initial cache state", "error", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go mgr.Start(ctx)

		h := handler.NewCASHandler(cacheDir, mgr)

		mux := http.NewServeMux()
		mux.Handle("/fetch/", h)

		addr := fmt.Sprintf(":%d", port)
		slog.Info("Starting server", "addr", addr, "cache_dir", cacheDir)

		server := &http.Server{
			Addr:    addr,
			Handler: mux,
		}

		if err := server.ListenAndServe(); err != nil {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().Int("port", 8080, "Port to run the server on")
	serverCmd.Flags().String("cache-dir", "./cache", "Directory to store cached files")
	serverCmd.Flags().Int64("max-cache-size", 1024*1024*1024, "Max cache size in bytes (default 1GB)")
	serverCmd.Flags().Int64("min-free-space", 0, "Min free disk space in bytes (if set, overrides max-cache-size)")
	serverCmd.Flags().Duration("eviction-interval", time.Minute, "Interval to check for evictions")
	serverCmd.Flags().String("eviction-strategy", "lru", "Eviction strategy to use (lru)")

	viper.BindPFlag("port", serverCmd.Flags().Lookup("port"))
	viper.BindPFlag("cache-dir", serverCmd.Flags().Lookup("cache-dir"))
	viper.BindPFlag("max-cache-size", serverCmd.Flags().Lookup("max-cache-size"))
	viper.BindPFlag("min-free-space", serverCmd.Flags().Lookup("min-free-space"))
	viper.BindPFlag("eviction-interval", serverCmd.Flags().Lookup("eviction-interval"))
	viper.BindPFlag("eviction-strategy", serverCmd.Flags().Lookup("eviction-strategy"))
}
