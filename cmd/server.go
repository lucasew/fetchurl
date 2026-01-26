package cmd

import (
	"fmt"
	"log"
	"net/http"

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

		h := handler.NewCASHandler(cacheDir)

		mux := http.NewServeMux()
		mux.Handle("/fetch/", h)

		addr := fmt.Sprintf(":%d", port)
		log.Printf("Starting server on %s with cache dir %s", addr, cacheDir)

		server := &http.Server{
			Addr:    addr,
			Handler: mux,
		}

		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)

	serverCmd.Flags().Int("port", 8080, "Port to run the server on")
	serverCmd.Flags().String("cache-dir", "./cache", "Directory to store cached files")

	viper.BindPFlag("port", serverCmd.Flags().Lookup("port"))
	viper.BindPFlag("cache-dir", serverCmd.Flags().Lookup("cache-dir"))
}
