package proxy

import (
	"context"
	"net/url"

	"github.com/lucasew/fetchurl/internal/db"
)

// NewDBRule creates a Rule that queries the database for the hash of the URL.
// It uses the provided algo to look up the hash.
func NewDBRule(database *db.DB, algo string) Rule {
	return func(ctx context.Context, u *url.URL) *RuleResult {
		// We use the full URL string as the key.
		key := u.String()

		hash, found, err := database.Get(ctx, key, algo)
		if err != nil {
			// In case of error, return nil to allow fallback
			return nil
		}
		if !found {
			return nil
		}

		return &RuleResult{
			Algo: algo,
			Hash: hash,
		}
	}
}
