package fetcher

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lucasew/fetchurl/internal/errutil"
	"github.com/lucasew/fetchurl/internal/hashutil"
	"github.com/schollz/progressbar/v3"
	"github.com/shogo82148/go-sfv"
)

type Fetcher struct {
	Client  *http.Client
	Servers []string
}

type FetchOptions struct {
	Algo string
	Hash string
	URLs []string
	Out  io.Writer
}

func NewFetcher(client *http.Client, servers []string) *Fetcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &Fetcher{
		Client:  client,
		Servers: servers,
	}
}

func (f *Fetcher) Fetch(ctx context.Context, opts FetchOptions) error {
	if !hashutil.IsSupported(opts.Algo) {
		return fmt.Errorf("unsupported algorithm: %s", opts.Algo)
	}

	cw := &CountingWriter{Writer: opts.Out}

	// 1. Try Servers
	for _, server := range f.Servers {
		err := f.fetchFromServer(ctx, server, opts.Algo, opts.Hash, opts.URLs, cw)
		if err == nil {
			return nil
		}
		slog.Warn("Failed to fetch from server", "server", server, "error", err)
		if cw.N > 0 {
			return fmt.Errorf("failed during download from server (partial write): %w", err)
		}
	}

	// 2. Fallback to Direct Download
	for _, url := range opts.URLs {
		err := f.fetchDirect(ctx, url, opts.Algo, opts.Hash, cw)
		if err == nil {
			return nil
		}
		slog.Warn("Failed to fetch from source", "url", url, "error", err)
		if cw.N > 0 {
			return fmt.Errorf("failed during download from source (partial write): %w", err)
		}
	}

	return fmt.Errorf("failed to fetch file from any source")
}

type CountingWriter struct {
	Writer io.Writer
	N      int64
}

func (c *CountingWriter) Write(p []byte) (n int, err error) {
	n, err = c.Writer.Write(p)
	c.N += int64(n)
	return n, err
}

func (f *Fetcher) fetchFromServer(ctx context.Context, server, algo, hashStr string, sourceUrls []string, out io.Writer) error {
	base := strings.TrimRight(server, "/")
	u := fmt.Sprintf("%s/api/fetchurl/%s/%s", base, algo, hashStr)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}

	if len(sourceUrls) > 0 {
		list := make(sfv.List, len(sourceUrls))
		for i, url := range sourceUrls {
			list[i] = sfv.Item{Value: url}
		}
		val, err := sfv.EncodeList(list)
		if err != nil {
			return fmt.Errorf("failed to encode X-Source-Urls: %w", err)
		}
		req.Header.Set("X-Source-Urls", val)
	}

	return f.doRequest(req, algo, hashStr, out)
}

func (f *Fetcher) fetchDirect(ctx context.Context, url, algo, hashStr string, out io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	return f.doRequest(req, algo, hashStr, out)
}

func (f *Fetcher) doRequest(req *http.Request, algo, expectedHash string, out io.Writer) error {
	resp, err := f.Client.Do(req)
	if err != nil {
		return err
	}
	defer errutil.LogMsg(resp.Body.Close(), "Failed to close response body")

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	bar := progressbar.NewOptions64(
		resp.ContentLength,
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionSetDescription("downloading"),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetWidth(10),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stderr, "\n")
		}),
	)

	hasher, err := hashutil.GetHasher(algo)
	if err != nil {
		return err
	}
	mw := io.MultiWriter(out, hasher, bar)

	if _, err := io.Copy(mw, resp.Body); err != nil {
		return err
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}
