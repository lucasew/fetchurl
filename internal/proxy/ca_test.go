package proxy_test

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/lucasew/fetchurl/internal/proxy"
)

func TestGenerateCA(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "fetchurl-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	certPath := filepath.Join(tempDir, "ca.pem")
	keyPath := filepath.Join(tempDir, "ca-key.pem")

	// Generate CA
	err = proxy.GenerateCA(certPath, keyPath)
	if err != nil {
		t.Fatalf("GenerateCA failed: %v", err)
	}

	// Verify files exist
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Errorf("Certificate file not created")
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Errorf("Key file not created")
	}

	// Verify we can load them as a valid keypair
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("Failed to load generated keypair: %v", err)
	}

	if len(cert.Certificate) == 0 {
		t.Errorf("No certificates loaded")
	}
}
