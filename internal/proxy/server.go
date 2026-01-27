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
		res := rule(r.Context(), r.URL)
		if res != nil {
			algo, hash := res.Algo, res.Hash
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

	resp.Header.Set("Cache-Control", "public, max-age=31536000, immutable")

	return resp
}
