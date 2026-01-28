package proxy

import (
	"context"
	"net/url"

	"github.com/lucasew/fetchurl/internal/db"
)

// NewDBMultiRule creates a Rule that queries the database for all hashes of a URL.
// Returns results ordered by priority (SHA256, SHA512, SHA1, others).
func NewDBMultiRule(database *db.DB) Rule {
	return func(ctx context.Context, u *url.URL) []RuleResult {
		key := u.String()

		hashes, err := database.GetAll(ctx, key)
		if err != nil || len(hashes) == 0 {
			return nil
		}

		// Database query already returns ordered by priority via SQL
		var results []RuleResult
		for _, h := range hashes {
			results = append(results, RuleResult{
				Algo: h.Algo,
				Hash: h.Hash,
			})
		}

		return results
	}
}
