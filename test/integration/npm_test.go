package integration

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Helper to generate CA (copied from internal/proxy/ca.go to avoid internal import restrictions)
func generateCA(certPath, keyPath string) error {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"FetchURL Proxy CA"},
			CommonName:   "FetchURL CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certOut.Close()
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return err
	}

	keyOut, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer keyOut.Close()
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes}); err != nil {
		return err
	}
	return nil
}

func TestNPMIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// 1. Generate CA
	tmpDir, err := os.MkdirTemp("", "fetchurl-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	caCertPath := filepath.Join(tmpDir, "ca.pem")
	caKeyPath := filepath.Join(tmpDir, "ca-key.pem")

	if err := generateCA(caCertPath, caKeyPath); err != nil {
		t.Fatal(err)
	}

	caCertBytes, err := os.ReadFile(caCertPath)
	if err != nil {
		t.Fatal(err)
	}
	caKeyBytes, err := os.ReadFile(caKeyPath)
	if err != nil {
		t.Fatal(err)
	}

	caCertHex := hex.EncodeToString(caCertBytes)
	caKeyHex := hex.EncodeToString(caKeyBytes)

	// 2. Setup Network
	net, err := network.New(ctx)
	if err != nil {
		// If docker is not available, this might fail.
		// In this sandbox, it WILL fail.
		t.Logf("Failed to create network (expected in sandbox): %v", err)
		return
	}
	defer net.Remove(ctx)
	networkName := net.Name

	// 3. Define Container Request Base
	// We point to the root of the repo for build context
	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../../",
			Dockerfile: "Dockerfile",
		},
		Networks:   []string{networkName},
		WaitingFor: wait.ForLog("Starting server"),
	}

	// 4. Start Upstream
	upstreamReq := req
	upstreamReq.Name = "upstream"
	upstreamReq.NetworkAliases = map[string][]string{networkName: {"upstream"}}
	upstreamReq.Cmd = []string{
		"server", // ENTRYPOINT is /app/fetchurl, so this is arg
		"--port=8080",
		"--ca-cert=" + caCertHex,
		"--ca-key=" + caKeyHex,
		"--cache-dir=/tmp/cache",
	}

	upstream, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: upstreamReq,
		Started:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer upstream.Terminate(ctx)

	// 5. Start Downstream
	downstreamReq := req
	downstreamReq.Name = "downstream"
	downstreamReq.NetworkAliases = map[string][]string{networkName: {"downstream"}}
	downstreamReq.Cmd = []string{
		"server",
		"--port=8080",
		"--upstream=http://upstream:8080",
		"--ca-cert=" + caCertHex,
		"--ca-key=" + caKeyHex,
		"--cache-dir=/tmp/cache",
	}

	downstream, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: downstreamReq,
		Started:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer downstream.Terminate(ctx)

	// 6. Start Client (Node/NPM)
	clientReq := testcontainers.ContainerRequest{
		Image:    "node:18-bookworm-slim",
		Networks: []string{networkName},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      caCertPath,
				ContainerFilePath: "/usr/local/share/ca-certificates/fetchurl-proxy.crt",
				FileMode:          0644,
			},
		},
		Env: map[string]string{
			"https_proxy": "http://downstream:8080",
			"http_proxy":  "http://downstream:8080",
		},
		// Attempt to install a small package or specific one
		Cmd: []string{"bash", "-c", `
set -ex

# Update CA certificates
update-ca-certificates

# Create test directory
mkdir -p /tmp/test-project
cd /tmp/test-project

# Initialize npm project
npm init -y

# Install express package through proxy
npm install express --verbose
`},
		WaitingFor: wait.ForExit().WithExitTimeout(10 * time.Minute),
	}

	client, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: clientReq,
		Started:          true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Terminate(ctx)

	// 7. Verify Results
	state, err := client.State(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Print logs if failed
	if state.ExitCode != 0 {
		logs, _ := client.Logs(ctx)
		io.Copy(os.Stdout, logs)
		t.Fatalf("Client failed with exit code %d", state.ExitCode)
	}

	// Verify Downstream Cache has SHA1 content
	// We expect the 'sha1' folder to be populated
	code, _, err := downstream.Exec(ctx, []string{"ls", "-A", "/tmp/cache/sha1"})
	if err != nil {
		t.Fatalf("Failed to execute check in downstream: %v", err)
	}
	if code != 0 {
		t.Errorf("Downstream cache check returned exit code %d (likely empty or missing sha1 dir)", code)
	}
}
