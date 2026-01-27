package main

import (
	"log/slog"
	"os"

	"github.com/lucasew/fetchurl/internal/proxy"
	"github.com/spf13/cobra"
)

var certCmd = &cobra.Command{
	Use:   "cert",
	Short: "Generates CA certificate and key",
	Run: func(cmd *cobra.Command, args []string) {
		outCert, err := cmd.Flags().GetString("out-cert")
		if err != nil {
			slog.Error("Failed to get out-cert flag", "error", err)
			os.Exit(1)
		}
		outKey, err := cmd.Flags().GetString("out-key")
		if err != nil {
			slog.Error("Failed to get out-key flag", "error", err)
			os.Exit(1)
		}

		slog.Info("Generating CA certificate and key", "cert", outCert, "key", outKey)
		if err := proxy.GenerateCA(outCert, outKey); err != nil {
			slog.Error("Failed to generate CA", "error", err)
			os.Exit(1)
		}
		slog.Info("Successfully generated CA certificate and key")
	},
}

func init() {
	rootCmd.AddCommand(certCmd)

	certCmd.Flags().String("out-cert", "ca.pem", "Output path for the CA certificate")
	certCmd.Flags().String("out-key", "ca-key.pem", "Output path for the CA private key")
}
