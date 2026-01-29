package proxy

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"

	"github.com/lucasew/fetchurl/internal/db"
)

// NPM types for parsing metadata
type npmDist struct {
	Tarball string `json:"tarball"`
	Shasum  string `json:"shasum"`
}

type npmVersion struct {
	Dist npmDist `json:"dist"`
}

type npmMetadata struct {
	Versions map[string]npmVersion `json:"versions"`
}

// NewNpmLearningRule creates a Rule that learns NPM package metadata.
// It detects requests to registry.npmjs.org, fetches metadata inline,
// extracts tarball URLs and SHA1 hashes, and inserts them into the database.
// Returns nil to pass the request through normally.
func NewNpmLearningRule(database *db.DB, client *http.Client) Rule {
	if client == nil {
		client = http.DefaultClient
	}

	registryRegex := regexp.MustCompile(`^https?://registry\.npmjs\.org/[^/]+/?$`)

	return func(ctx context.Context, u *url.URL) []RuleResult {
		// Verify this is an NPM registry metadata URL
		if !registryRegex.MatchString(u.String()) {
			return nil  // Not an NPM metadata URL
		}

		slog.Debug("NPM learning rule matched", "url", u.String())

		// Fetch metadata inline
		resp, err := client.Get(u.String())
		if err != nil {
			slog.Debug("Failed to fetch NPM metadata", "url", u.String(), "error", err)
			return nil  // Failed to fetch, let request pass through
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			slog.Debug("NPM metadata returned non-200 status", "url", u.String(), "status", resp.StatusCode)
			return nil  // Not a successful response
		}

		// Read and parse JSON
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Debug("Failed to read NPM metadata body", "error", err)
			return nil
		}

		var metadata npmMetadata
		if err := json.Unmarshal(body, &metadata); err != nil {
			slog.Debug("Failed to parse NPM metadata JSON", "error", err)
			return nil
		}

		// Extract and insert URL→hash mappings into database
		count := 0
		bgCtx := context.Background()  // Use background context to avoid cancellation
		for _, ver := range metadata.Versions {
			if ver.Dist.Tarball != "" && ver.Dist.Shasum != "" {
				err := database.InsertHash(bgCtx, db.InsertHashParams{
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
			slog.Info("Learned NPM artifacts", "count", count, "package", u.Path)
		}

		// Return nil - request passes through normally
		return nil
	}
}
