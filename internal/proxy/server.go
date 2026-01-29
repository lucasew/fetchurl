package proxy

import (
	"crypto/tls"
	"io"
	"log/slog"
	"net/http"

	"github.com/elazarl/goproxy"
	"github.com/lucasew/fetchurl/internal/fetcher"
	"github.com/lucasew/fetchurl/internal/repository"
)

type Server struct {
	Proxy   *goproxy.ProxyHttpServer
	Local   repository.WritableRepository
	Fetcher fetcher.Fetcher
	Rules   []Rule
}

// NewServer creates a new Proxy Server.
// fallback is the handler to use for non-proxy requests (e.g. local routes).
func NewServer(local repository.WritableRepository, fetcher fetcher.Fetcher, rules []Rule, fallback http.Handler, caCert *tls.Certificate) *Server {
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true

	if caCert != nil {
		proxy.OnRequest().HandleConnect(goproxy.FuncHttpsHandler(func(host string, ctx *goproxy.ProxyCtx) (*goproxy.ConnectAction, string) {
			return &goproxy.ConnectAction{
				Action:    goproxy.ConnectMitm,
				TLSConfig: goproxy.TLSConfigFromCA(caCert),
			}, host
		}))
	} else {
		proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
	}

	if fallback != nil {
		proxy.NonproxyHandler = fallback
	}

	s := &Server{
		Proxy:   proxy,
		Local:   local,
		Fetcher: fetcher,
		Rules:   rules,
	}

	proxy.OnRequest().DoFunc(s.handleRequest)
	return s
}

func (s *Server) handleRequest(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
	slog.Debug("request", "curl", ctx.Req.URL, "rurl", r.URL)
	for _, rule := range s.Rules {
		results := rule(r.Context(), r.URL)
		if len(results) == 0 {
			continue
		}

		slog.Info("Proxy rule matched", "url", r.URL.String(), "result_count", len(results))

		// Try each hash in order
		for _, res := range results {
			algo, hash := res.Algo, res.Hash
			slog.Debug("Trying hash", "algo", algo, "hash", hash)

			// Check if already in cache (before fetching)
			cacheReader, _, cacheErr := s.Local.Get(r.Context(), algo, hash)
			isCacheHit := cacheErr == nil
			if isCacheHit {
				cacheReader.Close() // We'll get it again via GetOrFetch
				slog.Info("Cache HIT", "url", r.URL.String(), "algo", algo, "hash", hash)
			} else {
				slog.Info("Cache MISS", "url", r.URL.String(), "algo", algo, "hash", hash)
			}

			fetchFn := func() (io.ReadCloser, int64, error) {
				return s.Fetcher.Fetch(r.Context(), algo, hash, []string{r.URL.String()})
			}

			reader, size, err := s.Local.GetOrFetch(r.Context(), algo, hash, fetchFn)
			if err != nil {
				slog.Warn("Failed to fetch with hash, trying next", "algo", algo, "hash", hash, "error", err)
				continue  // Try next hash
			}
			slog.Info("Proxy served", "url", r.URL.String(), "algo", algo, "hash", hash, "cache_hit", isCacheHit)
			return r, s.newResponse(r, reader, size, algo, hash)
		}

		// All hashes from this rule failed
		slog.Warn("All hashes failed for matched rule", "url", r.URL.String())
		return r, nil  // Fallback to normal proxy
	}

	// No rule matched, pass through
	return r, nil
}

func (s *Server) newResponse(r *http.Request, body io.ReadCloser, size int64, algo, hash string) *http.Response {
	// 200 OK
	resp := goproxy.NewResponse(r, "application/octet-stream", http.StatusOK, "")
	resp.Body = body
	resp.ContentLength = size

	resp.Header.Set("Cache-Control", "public, max-age=31536000, immutable")

	return resp
}
