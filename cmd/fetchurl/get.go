package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/lucasew/fetchurl"
	"github.com/lucasew/fetchurl/internal/errutil"
	"github.com/schollz/progressbar/v3"
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

		f := fetchurl.NewFetcher(client, servers)

		var out io.Writer
		if output != "" {
			file, err := os.Create(output)
			if err != nil {
				errutil.ReportError(err, "Failed to create output file")
				os.Exit(1)
			}
			defer func() {
				errutil.LogMsg(file.Close(), "Failed to close output file")
			}()
			out = file
		} else {
			out = os.Stdout
		}

		bar := progressbar.NewOptions64(
			-1,
			progressbar.OptionSetWriter(os.Stderr),
			progressbar.OptionSetDescription("downloading"),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetWidth(10),
			progressbar.OptionThrottle(65*time.Millisecond),
			progressbar.OptionOnCompletion(func() {
				fmt.Fprint(os.Stderr, "\n")
			}),
		)

		if err := f.Fetch(cmd.Context(), fetchurl.FetchOptions{
			Algo: algo,
			Hash: hash,
			URLs: urls,
			Out:  io.MultiWriter(out, bar),
		}); err != nil {
			errutil.ReportError(err, "Fetch failed")
			if output != "" {
				errutil.LogMsg(os.Remove(output), "Failed to remove output file after failed fetch", "path", output)
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
