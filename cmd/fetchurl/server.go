package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

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
		upstreams := viper.GetStringSlice("upstream")

		h := handler.NewCASHandler(cacheDir, upstreams)

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
	serverCmd.Flags().StringSlice("upstream", []string{}, "List of upstream fetchurl servers")

	viper.BindPFlag("port", serverCmd.Flags().Lookup("port"))
	viper.BindPFlag("cache-dir", serverCmd.Flags().Lookup("cache-dir"))
	viper.BindPFlag("upstream", serverCmd.Flags().Lookup("upstream"))
}
