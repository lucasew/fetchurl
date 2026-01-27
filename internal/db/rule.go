package db

import (
	"context"
	"net/url"

	"github.com/lucasew/fetchurl/internal/proxy"
)

// NewRule creates a Rule that queries the database for the hash of the URL.
func NewRule(db *DB, algo string) proxy.Rule {
	return func(u *url.URL) *proxy.RuleResult {
		// We use the full URL string as the key.
		key := u.String()

		hash, found, err := db.Get(context.Background(), key)
		if err != nil {
			// In case of error, return nil to allow fallback
			return nil
		}
		if !found {
			return nil
		}

		return &proxy.RuleResult{
			Algo: algo,
			Hash: hash,
		}
	}
}
