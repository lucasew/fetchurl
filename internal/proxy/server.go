package proxy

import (
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

func NewServer(local repository.WritableRepository, fetcher fetcher.Fetcher, rules []Rule) *Server {
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true // Useful for debugging, maybe make configurable later

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
	for _, rule := range s.Rules {
		algo, hash, match := rule.Match(r)
		if match {
			slog.Info("Proxy rule matched", "url", r.URL.String(), "algo", algo, "hash", hash)

			// 1. Try Local Cache (HIT)
			reader, size, err := s.Local.Get(r.Context(), algo, hash)
			if err == nil {
				slog.Info("Proxy cache hit", "algo", algo, "hash", hash)
				return r, s.newResponse(r, reader, size, algo, hash)
			}

			// 2. Cache Miss -> Fetch & Store
			slog.Info("Proxy cache miss, fetching", "algo", algo, "hash", hash)

			// The fetch function for Local.Put.
			// We pass the original request URL as the source.
			// s.Fetcher (Service) will also check upstreams if configured.
			fetchFn := func() (io.ReadCloser, int64, error) {
				return s.Fetcher.Fetch(r.Context(), algo, hash, []string{r.URL.String()})
			}

			err = s.Local.Put(r.Context(), algo, hash, fetchFn)
			if err != nil {
				slog.Warn("Failed to fetch/store in proxy, falling back to direct proxy", "error", err)
				// Fallback: return nil response, goproxy will proxy the request normally
				return r, nil
			}

			// 3. Serve after successful Store
			reader, size, err = s.Local.Get(r.Context(), algo, hash)
			if err != nil {
				slog.Error("Failed to retrieve after store in proxy", "error", err)
				return r, nil
			}
			return r, s.newResponse(r, reader, size, algo, hash)
		}
	}

	// No rule matched, pass through
	return r, nil
}

func (s *Server) newResponse(r *http.Request, body io.ReadCloser, size int64, algo, hash string) *http.Response {
	// 200 OK
	resp := goproxy.NewResponse(r, "application/octet-stream", http.StatusOK, "")
	resp.Body = body
	resp.ContentLength = size

	// Set headers similar to CASHandler
	resp.Header.Set("Cache-Control", "public, max-age=31536000, immutable")
	// We don't easily know the canonical CAS URL here, so we skip the Link header for now
	// unless we want to reconstruct it assuming the CAS server is running on the same host?
	// Better to leave it out than be wrong.

	return resp
}
