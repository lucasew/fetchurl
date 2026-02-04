package main

import (
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/lucasew/fetchurl/internal/fetcher"
	"github.com/shogo82148/go-sfv"
	"github.com/spf13/cobra"
)

var getCmd = &cobra.Command{
	Use:   "get <algo> <hash>",
	Short: "Fetch a file using CAS",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		algo := args[0]
		hash := args[1]
		urls, _ := cmd.Flags().GetStringSlice("url")
		output, _ := cmd.Flags().GetString("output")

		// Parse FETCHURL_SERVER
		var servers []string
		envServer := os.Getenv("FETCHURL_SERVER")
		if envServer != "" {
			list, err := sfv.DecodeList([]string{envServer})
			if err != nil {
				slog.Warn("Failed to parse FETCHURL_SERVER", "error", err)
			} else {
				for _, item := range list {
					if s, ok := item.Value.(string); ok {
						servers = append(servers, s)
					}
				}
			}
		}

		client := http.DefaultClient

		f := fetcher.NewFetcher(client, servers)

		var out io.Writer
		if output != "" {
			file, err := os.Create(output)
			if err != nil {
				slog.Error("Failed to create output file", "error", err)
				os.Exit(1)
			}
			defer file.Close()
			out = file
		} else {
			out = os.Stdout
		}

		if err := f.Fetch(cmd.Context(), algo, hash, urls, out); err != nil {
			slog.Error("Fetch failed", "error", err)
			if output != "" {
				os.Remove(output)
			}
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(getCmd)
	getCmd.Flags().StringSlice("url", []string{}, "Source URLs")
	getCmd.Flags().StringP("output", "o", "", "Output file")
}
