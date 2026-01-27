package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/elazarl/goproxy"
	"github.com/lucasew/fetchurl/internal/db"
)

type NpmDist struct {
	Tarball string `json:"tarball"`
	Shasum  string `json:"shasum"`
}

type NpmVersion struct {
	Dist NpmDist `json:"dist"`
}

type NpmMetadata struct {
	Versions map[string]NpmVersion `json:"versions"`
}

var npmRegistryRegex = regexp.MustCompile(`^https?://registry\.npmjs\.org/`)

func NewNpmResponseHandler(queries *db.Queries) goproxy.RespHandler {
	return goproxy.FuncRespHandler(func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
		// Only check successful responses
		if resp == nil || resp.StatusCode != http.StatusOK {
			return resp
		}

		// Check URL
		if !npmRegistryRegex.MatchString(ctx.Req.URL.String()) {
			return resp
		}

		// Check Content-Type
		contentType := resp.Header.Get("Content-Type")
		if !strings.Contains(contentType, "application/json") {
			return resp
		}

		// Read Body
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Error("Failed to read NPM response body", "error", err)
			return resp
		}
		resp.Body.Close()

		// Restore body for client
		resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Parse and Learn
		var metadata NpmMetadata
		if err := json.Unmarshal(bodyBytes, &metadata); err != nil {
			// Not a fatal error for the proxy, just can't learn from it
			slog.Debug("Failed to parse NPM metadata", "url", ctx.Req.URL.String(), "error", err)
			return resp
		}

		count := 0
		bgCtx := context.Background() // Use background context to avoid cancellation if request ends
		for _, ver := range metadata.Versions {
			if ver.Dist.Tarball != "" && ver.Dist.Shasum != "" {
				err := queries.InsertHash(bgCtx, db.InsertHashParams{
					Url:  ver.Dist.Tarball,
					Hash: ver.Dist.Shasum,
					Algo: "sha1",
				})
				if err != nil {
					slog.Debug("Failed to insert NPM hash", "url", ver.Dist.Tarball, "error", err)
				} else {
					count++
				}
			}
		}

		if count > 0 {
			slog.Info("Learned NPM artifacts", "count", count, "pkg", ctx.Req.URL.Path)
		}

		return resp
	})
}
