package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/lucasew/fetchurl/internal/eviction"
	"github.com/lucasew/fetchurl/internal/eviction/lru"
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
		evictionInterval := viper.GetDuration("eviction-interval")
		evictionStrategy := viper.GetString("eviction-strategy")

		// Setup Eviction Manager
		var strat eviction.Strategy
		switch evictionStrategy {
		case "lru":
			strat = lru.New()
		default:
			slog.Warn("Unknown eviction strategy, defaulting to LRU", "strategy", evictionStrategy)
			strat = lru.New()
		}

		mgr := eviction.NewManager(cacheDir, maxCacheSize, evictionInterval, strat)

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
	serverCmd.Flags().Duration("eviction-interval", time.Minute, "Interval to check for evictions")
	serverCmd.Flags().String("eviction-strategy", "lru", "Eviction strategy to use (lru)")

	viper.BindPFlag("port", serverCmd.Flags().Lookup("port"))
	viper.BindPFlag("cache-dir", serverCmd.Flags().Lookup("cache-dir"))
	viper.BindPFlag("max-cache-size", serverCmd.Flags().Lookup("max-cache-size"))
	viper.BindPFlag("eviction-interval", serverCmd.Flags().Lookup("eviction-interval"))
	viper.BindPFlag("eviction-strategy", serverCmd.Flags().Lookup("eviction-strategy"))
}
