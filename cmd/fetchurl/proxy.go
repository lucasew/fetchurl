package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/lucasew/fetchurl/internal/app"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var proxyCmd = &cobra.Command{
	Use:   "proxy",
	Short: "Starts the CAS Proxy Server",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := app.Config{
			Port:             viper.GetInt("proxy-port"),
			CacheDir:         viper.GetString("cache-dir"),
			MaxCacheSize:     viper.GetInt64("max-cache-size"),
			MinFreeSpace:     viper.GetInt64("min-free-space"),
			EvictionInterval: viper.GetDuration("eviction-interval"),
			EvictionStrategy: viper.GetString("eviction-strategy"),
			Upstreams:        viper.GetStringSlice("upstream"),
		}

		server, cleanup, err := app.NewProxyServer(cfg)
		if err != nil {
			slog.Error("Failed to initialize proxy server", "error", err)
			os.Exit(1)
		}
		defer cleanup()

		if err := server.ListenAndServe(); err != nil {
			slog.Error("Proxy server failed", "error", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(proxyCmd)

	proxyCmd.Flags().Int("proxy-port", 8081, "Port to run the proxy on")
	proxyCmd.Flags().String("cache-dir", "./cache", "Directory to store cached files")
	proxyCmd.Flags().Int64("max-cache-size", 1024*1024*1024, "Max cache size in bytes (default 1GB)")
	proxyCmd.Flags().Int64("min-free-space", 0, "Min free disk space in bytes (if set, overrides max-cache-size)")
	proxyCmd.Flags().Duration("eviction-interval", time.Minute, "Interval to check for evictions")
	proxyCmd.Flags().String("eviction-strategy", "lru", "Eviction strategy to use (lru)")
	proxyCmd.Flags().StringSlice("upstream", []string{}, "Upstream CAS servers")

	mustBindPFlag("proxy-port", proxyCmd.Flags().Lookup("proxy-port"))
	mustBindPFlag("cache-dir", proxyCmd.Flags().Lookup("cache-dir"))
	mustBindPFlag("max-cache-size", proxyCmd.Flags().Lookup("max-cache-size"))
	mustBindPFlag("min-free-space", proxyCmd.Flags().Lookup("min-free-space"))
	mustBindPFlag("eviction-interval", proxyCmd.Flags().Lookup("eviction-interval"))
	mustBindPFlag("eviction-strategy", proxyCmd.Flags().Lookup("eviction-strategy"))
	mustBindPFlag("upstream", proxyCmd.Flags().Lookup("upstream"))
}
