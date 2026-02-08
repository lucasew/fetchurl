package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sync/errgroup"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

type Package struct {
	Name string
	URL  string
	Algo string
	Hash string
}

var packages = []Package{
	{"lodash", "https://registry.npmjs.org/lodash/-/lodash-4.17.23.tgz", "sha1", "f113b0378386103be4f6893388c73d0bde7f2c5a"},
	{"chalk", "https://registry.npmjs.org/chalk/-/chalk-5.6.2.tgz", "sha1", "b1238b6e23ea337af71c7f8a295db5af0c158aea"},
	{"debug", "https://registry.npmjs.org/debug/-/debug-4.4.3.tgz", "sha1", "c6ae432d9bd9662582fce08709b038c58e9e3d6a"},
	{"commander", "https://registry.npmjs.org/commander/-/commander-14.0.3.tgz", "sha1", "425d79b48f9af82fcd9e4fc1ea8af6c5ec07bbc2"},
	{"express", "https://registry.npmjs.org/express/-/express-5.2.1.tgz", "sha1", "8f21d15b6d327f92b4794ecf8cb08a72f956ac04"},
	{"react", "https://registry.npmjs.org/react/-/react-19.2.4.tgz", "sha1", "438e57baa19b77cb23aab516cf635cd0579ee09a"},
	{"vue", "https://registry.npmjs.org/vue/-/vue-3.5.27.tgz", "sha1", "e55fd941b614459ab2228489bc19d1692e05876c"},
	{"typescript", "https://registry.npmjs.org/typescript/-/typescript-5.9.3.tgz", "sha1", "5b4f59e15310ab17a216f5d6cf53ee476ede670f"},
	{"uuid", "https://registry.npmjs.org/uuid/-/uuid-13.0.0.tgz", "sha1", "263dc341b19b4d755eb8fe36b78d95a6b65707e8"},
	{"axios", "https://registry.npmjs.org/axios/-/axios-1.13.4.tgz", "sha1", "15d109a4817fb82f73aea910d41a2c85606076bc"},
}

func TestSDKIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// 1. Setup Network
	net, err := network.New(ctx)
	if err != nil {
		t.Fatalf("failed to create network: %v", err)
	}
	defer net.Remove(ctx)

	networkName := net.Name

	// 2. Resolve Repo Root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// wd is .../test/integration, go up 2 levels
	repoRoot := filepath.Dir(filepath.Dir(wd))

	// 3. Start FetchURL Server
	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context: repoRoot,
			Dockerfile: "Dockerfile",
		},
		ExposedPorts: []string{"8080/tcp"},
		Networks:     []string{networkName},
		NetworkAliases: map[string][]string{
			networkName: {"fetchurl-server"},
		},
		WaitingFor: wait.ForLog("Listening on"), // Adjust based on server startup log
		Env: map[string]string{
			"FETCHURL_PORT": "8080",
		},
	}
    // Note: The server logs "Listening on :8080" or similar?
    // I should check server logs or just wait for port.
    // Given I don't know the exact log line, I'll wait for the port.
    req.WaitingFor = wait.ForListeningPort("8080/tcp")

	serverContainer, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start server container: %v", err)
	}
	defer serverContainer.Terminate(ctx)

	// Helper to run client
	runClient := func(sdk string, pkg Package) error {
		var cmd []string
		var workingDir string
		switch sdk {
		case "go":
			workingDir = "/src"
			cmd = []string{"mise", "exec", "-y", "--", "go", "run", "./cmd/fetchurl", "get", pkg.Algo, pkg.Hash, "--url", pkg.URL, "-o", "/dev/null"}
		case "rust":
			workingDir = "/src/sdk/rust"
			cmd = []string{"mise", "exec", "-y", "--", "cargo", "run", "--example", "get", "--", pkg.Algo, pkg.Hash, "--url", pkg.URL, "-o", "/dev/null"}
		case "js":
			workingDir = "/src"
			cmd = []string{"mise", "exec", "-y", "--", "node", "sdk/js/cli.js", pkg.Algo, pkg.Hash, "--url", pkg.URL, "-o", "/dev/null"}
		case "python":
			workingDir = "/src/sdk/python"
			cmd = []string{"mise", "exec", "-y", "--", "python3", "-m", "fetchurl", pkg.Algo, pkg.Hash, "--url", pkg.URL, "-o", "/dev/null"}
		default:
			return fmt.Errorf("unknown sdk: %s", sdk)
		}

        // Quote the server URL for RFC 8941?
        // FETCHURL_SERVER is a string list.
        // If I pass "http://fetchurl-server:8080", sfv parser handles it.
        // It needs quotes if it contains special chars, but http://... is usually fine as token?
        // No, URLs must be quoted strings in SFV.
        // So env var should be `"http://fetchurl-server:8080"`.
		env := map[string]string{
			"FETCHURL_SERVER": "\"http://fetchurl-server:8080\"",
		}

		req := testcontainers.ContainerRequest{
			Image:      "jdxcode/mise:latest",
			Networks:   []string{networkName},
			Env:        env,
			Cmd:        cmd,
			WorkingDir: workingDir,
			Mounts: testcontainers.ContainerMounts{
				testcontainers.BindMount(repoRoot, "/src"),
			},
            // Start and wait for exit
            WaitingFor: wait.ForExit(),
		}

		c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
		if err != nil {
			return fmt.Errorf("[%s] failed to start container: %v", sdk, err)
		}
		defer c.Terminate(ctx)

		state, err := c.State(ctx)
		if err != nil {
			return fmt.Errorf("[%s] failed to get state: %v", sdk, err)
		}
		if state.ExitCode != 0 {
			// Need to read logs to debug
			// testcontainers-go logs handling is a bit verbose, skipping for brevity unless needed
			return fmt.Errorf("[%s] exited with code %d. Logs might be available if attached.", sdk, state.ExitCode)
		}
		return nil
	}

	sdks := []string{"go", "rust", "js", "python"}

	t.Run("SimultaneousSameFile", func(t *testing.T) {
		pkg := packages[0]
		wg := new(errgroup.Group)
		for _, sdk := range sdks {
			sdk := sdk
			wg.Go(func() error {
				return runClient(sdk, pkg)
			})
		}
		if err := wg.Wait(); err != nil {
			t.Error(err)
		}
	})

	t.Run("SimultaneousDifferentFiles", func(t *testing.T) {
		wg := new(errgroup.Group)
		for i, sdk := range sdks {
			if i >= len(packages) {
				break
			}
			pkg := packages[i+1] // Use different packages (offset 1)
			sdk := sdk
			wg.Go(func() error {
				return runClient(sdk, pkg)
			})
		}
		if err := wg.Wait(); err != nil {
			t.Error(err)
		}
	})
}
