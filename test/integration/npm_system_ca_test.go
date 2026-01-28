package integration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestNPMWithSystemCA tests the complete integration flow with CA installed in system truststore.
// This validates the real-world scenario where the proxy intercepts ALL HTTPS traffic transparently.
// The CA is installed using update-ca-certificates, making it trusted by the entire system,
// not just specific tools like Node.js.
func TestNPMWithSystemCA(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := t.Context()

	// 1. Generate CA certificate
	tmpDir, err := os.MkdirTemp("", "fetchurl-system-ca-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	caCertPath := filepath.Join(tmpDir, "ca.crt")
	caKeyPath := filepath.Join(tmpDir, "ca.key")

	if err := generateCA(caCertPath, caKeyPath); err != nil {
		t.Fatalf("Failed to generate CA: %v", err)
	}

	// 2. Create Docker network
	net, err := network.New(ctx)
	if err != nil {
		t.Skipf("Docker not available (expected in some environments): %v", err)
		return
	}
	defer net.Remove(ctx)
	networkName := net.Name

	// 3. Start fetchurl proxy server
	proxyReq := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "../../",
			Dockerfile: "Dockerfile",
		},
		Networks: []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {"proxy"},
		},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      caCertPath,
				ContainerFilePath: "/tmp/ca.crt",
				FileMode:          0644,
			},
			{
				HostFilePath:      caKeyPath,
				ContainerFilePath: "/tmp/ca.key",
				FileMode:          0600,
			},
		},
		Cmd: []string{
			"server",
			"--port=8080",
			"--ca-cert=/tmp/ca.crt",
			"--ca-key=/tmp/ca.key",
			"--cache-dir=/cache",
		},
		WaitingFor: wait.ForLog("Starting server").WithStartupTimeout(30 * time.Second),
	}

	proxyContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: proxyReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start proxy container: %v", err)
	}
	defer func() {
		if t.Failed() {
			// Print logs on test failure
			logs, _ := proxyContainer.Logs(ctx)
			fmt.Println("\n=== Proxy Container Logs ===")
			io.Copy(os.Stdout, logs)
		}
		proxyContainer.Terminate(ctx)
	}()

	// 4. Start Node.js client with system-wide CA installation
	// Using Debian-based image for proper update-ca-certificates support
	clientReq := testcontainers.ContainerRequest{
		Image:    "node:18-bookworm-slim", // Debian-based for update-ca-certificates
		Networks: []string{networkName},
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      caCertPath,
				ContainerFilePath: "/usr/local/share/ca-certificates/fetchurl-proxy.crt",
				FileMode:          0644,
			},
		},
		Env: map[string]string{
			"HTTP_PROXY":  "http://proxy:8080",
			"HTTPS_PROXY": "http://proxy:8080",
			"http_proxy":  "http://proxy:8080",
			"https_proxy": "http://proxy:8080",
		},
		Cmd: []string{"bash", "-c", `
set -ex

# Install CA certificate into system truststore
update-ca-certificates

# Verify CA was installed
ls /etc/ssl/certs/ | grep -i fetchurl || echo "Note: CA installed but symlink may not show separately"

# Create test directory
mkdir -p /tmp/test-project
cd /tmp/test-project

# Initialize npm project
npm init -y

# Install express (pulls from registry.npmjs.org, has dependencies)
npm install express --verbose 2>&1

# Verify installation succeeded
test -d node_modules/express || (echo "ERROR: express not installed" && exit 1)

echo "SUCCESS: NPM installation completed through proxy with system CA"
`},
		WaitingFor: wait.ForExit().WithExitTimeout(5 * time.Minute),
	}

	clientContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: clientReq,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("Failed to start client container: %v", err)
	}
	defer func() {
		// Always print client logs for debugging
		logs, _ := clientContainer.Logs(ctx)
		fmt.Println("\n=== Client Container Logs ===")
		io.Copy(os.Stdout, logs)
		clientContainer.Terminate(ctx)
	}()

	// 5. Verify client container completed successfully
	state, err := clientContainer.State(ctx)
	if err != nil {
		t.Fatalf("Failed to get client state: %v", err)
	}

	if state.ExitCode != 0 {
		t.Fatalf("Client npm installation failed with exit code %d", state.ExitCode)
	}

	t.Log("✅ Client npm installation succeeded")

	// 6. Verify proxy learned NPM hashes from registry.npmjs.org
	exitCode, reader, err := proxyContainer.Exec(ctx, []string{
		"sqlite3", "/cache/links.db",
		"SELECT COUNT(*) FROM urls WHERE url LIKE '%registry.npmjs.org%' AND algo = 'sha1';",
	})
	if err != nil {
		t.Fatalf("Failed to query proxy database: %v", err)
	}

	if exitCode != 0 {
		t.Errorf("Database query failed with exit code %d", exitCode)
	}

	output, _ := io.ReadAll(reader)
	hashCount := strings.TrimSpace(string(output))
	t.Logf("NPM hashes learned by proxy: %s", hashCount)

	// Verify proxy actually learned something
	if hashCount == "0" || hashCount == "" {
		t.Error("Expected proxy to learn NPM package hashes, but database is empty")
	}

	// 7. Verify proxy has cached SHA1 tarballs
	exitCode, reader, err = proxyContainer.Exec(ctx, []string{
		"sh", "-c", "ls -1 /cache/sha1 2>/dev/null | wc -l",
	})
	if err != nil {
		t.Fatalf("Failed to list cache files: %v", err)
	}

	output, _ = io.ReadAll(reader)
	fileCount := strings.TrimSpace(string(output))
	t.Logf("SHA1-cached tarballs in proxy: %s", fileCount)

	if fileCount == "0" || fileCount == "" {
		t.Error("Expected proxy to have cached SHA1 tarballs, but cache directory is empty")
	}

	t.Log("✅ Integration test passed: NPM installation worked through proxy with system CA, learning and caching hashes")
}
