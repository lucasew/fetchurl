package integration

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lucasew/fetchurl/internal/proxy"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestProxyIntegration(t *testing.T) {
	ctx := context.Background()

	// 1. Generate CA
	tempDir, err := os.MkdirTemp("", "fetchurl-integration")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	certPath := filepath.Join(tempDir, "ca.pem")
	keyPath := filepath.Join(tempDir, "ca-key.pem")

	if err := proxy.GenerateCA(certPath, keyPath); err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	caCertContent, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("Failed to read CA cert: %v", err)
	}
	caKeyContent, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("Failed to read CA key: %v", err)
	}

	// 2. Build Docker Image (or use context)
	// For this test, we assume we can build from the current directory.
	// In a CI environment, we might need a pre-built image.
	// Since we are running `go test`, we can try to use `FromDockerfile`.

	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../../", // Root of repo
			Dockerfile: "Dockerfile",
			KeepImage:  true,
		},
		ExposedPorts: []string{"8080/tcp"},
		Env: map[string]string{
			"FETCHURL_PORT":            "8080",
			"FETCHURL_CA_CERT_CONTENT": string(caCertContent),
			"FETCHURL_CA_KEY_CONTENT":  string(caKeyContent),
		},
		WaitingFor: wait.ForLog("Starting server"),
		Cmd:        []string{"server"},
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}
	defer func() { _ = container.Terminate(ctx) }()

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "8080")
	if err != nil {
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s:%s", host, port.Port()))

	// 3. Configure HTTP Client to trust the CA
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCertContent)

	httpClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		},
		Timeout: 10 * time.Second,
	}

	// 4. Make a request via the proxy (MITM)
	// We'll request a known HTTPS site.
	// Note: In a completely isolated env, this might fail if no internet.
	// But `google.com` is a standard check.
	targetURL := "https://example.com"
	resp, err := httpClient.Get(targetURL)
	if err != nil {
		t.Fatalf("Failed to make request via proxy: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 OK, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Errorf("Empty response body")
	}

	t.Logf("Successfully proxied to %s via custom CA", targetURL)
}
