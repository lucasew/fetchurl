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

	"github.com/lucasew/fetchurl/internal/hashutil"
	"github.com/schollz/progressbar/v3"
	"github.com/shogo82148/go-sfv"
)

type Fetcher struct {
	Client  *http.Client
	Servers []string
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

func (f *Fetcher) Fetch(ctx context.Context, algo, hashStr string, urls []string, out io.Writer) error {
	if !hashutil.IsSupported(algo) {
		return fmt.Errorf("unsupported algorithm: %s", algo)
	}

	// 1. Try Servers
	for _, server := range f.Servers {
		err := f.fetchFromServer(ctx, server, algo, hashStr, urls, out)
		if err == nil {
			return nil
		}
		slog.Warn("Failed to fetch from server", "server", server, "error", err)
		f.resetOutput(out)
	}

	// 2. Fallback to Direct Download
	for _, url := range urls {
		err := f.fetchDirect(ctx, url, algo, hashStr, out)
		if err == nil {
			return nil
		}
		slog.Warn("Failed to fetch from source", "url", url, "error", err)
		f.resetOutput(out)
	}

	return fmt.Errorf("failed to fetch file from any source")
}

func (f *Fetcher) resetOutput(out io.Writer) {
	if seeker, ok := out.(io.Seeker); ok {
		_, _ = seeker.Seek(0, 0)
		if file, ok := out.(*os.File); ok {
			_ = file.Truncate(0)
		}
	}
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
		if err == nil {
			req.Header.Set("X-Source-Urls", val)
		}
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
	defer resp.Body.Close()

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

	hasher, _ := hashutil.GetHasher(algo)
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
