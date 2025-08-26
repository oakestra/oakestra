package virtualization

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsGCloudArtifactUrl(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "valid gcloud artifact url",
			url:      "https://artifactregistry.googleapis.com/download/v1/projects/tum-cm/locations/europe-west3/repositories/wasm/files/wasm:1.0.0:3mm_with_cr.wasm:download?alt=media",
			expected: true,
		},
		{
			name:     "invalid url - different domain",
			url:      "https://github.com/example/file.wasm",
			expected: false,
		},
		{
			name:     "invalid url - no https",
			url:      "http://artifactregistry.googleapis.com/download/v1/test",
			expected: false,
		},
		{
			name:     "empty url",
			url:      "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGCloudArtifactUrl(tt.url)
			if result != tt.expected {
				t.Errorf("isGCloudArtifactUrl(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestGetFileSHA256(t *testing.T) {
	// Create a temporary file with known content
	tempFile, err := os.CreateTemp("", "test-*.wasm")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	testContent := "test wasm content"
	if _, err := tempFile.WriteString(testContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tempFile.Close()

	// Calculate expected hash
	hasher := sha256.New()
	hasher.Write([]byte(testContent))
	expectedHash := hex.EncodeToString(hasher.Sum(nil))

	// Test the function
	actualHash, err := getFileSHA256(tempFile.Name())
	if err != nil {
		t.Fatalf("getFileSHA256() failed: %v", err)
	}

	if actualHash != expectedHash {
		t.Errorf("getFileSHA256() = %q, want %q", actualHash, expectedHash)
	}
}

func TestGetFileSHA256_NonExistentFile(t *testing.T) {
	_, err := getFileSHA256("/non/existent/file.wasm")
	if err == nil {
		t.Error("getFileSHA256() should fail for non-existent file")
	}
}

func TestDeleteWasmModule(t *testing.T) {
	// Create a temporary file
	tempFile, err := os.CreateTemp("", "test-*.wasm")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tempFile.Close()

	// Test deletion
	err = deleteWasmModule(tempFile.Name())
	if err != nil {
		t.Fatalf("deleteWasmModule() failed: %v", err)
	}

	// Verify file is deleted
	if _, err := os.Stat(tempFile.Name()); !os.IsNotExist(err) {
		t.Error("File should be deleted")
	}
}

func TestDeleteWasmModule_NonExistentFile(t *testing.T) {
	err := deleteWasmModule("/non/existent/file.wasm")
	if err == nil {
		t.Error("deleteWasmModule() should fail for non-existent file")
	}
}

func TestDownloadWasmModule_InvalidURL(t *testing.T) {
	invalidURLs := []string{
		"https://github.com/example/file.wasm",
		"invalid-url",
		"",
	}

	for _, url := range invalidURLs {
		t.Run(fmt.Sprintf("invalid_url_%s", url), func(t *testing.T) {
			_, err := downloadWasmModule(url)
			if err == nil {
				t.Errorf("downloadWasmModule(%q) should fail for invalid URL", url)
			}
		})
	}
}

func TestDownloadGCloudGenericArtifact_URLValidation(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "invalid scheme",
			url:         "ftp://artifactregistry.googleapis.com/download/v1/test:download?alt=media",
			shouldError: true,
			errorMsg:    "invalid URL scheme",
		},
		{
			name:        "invalid host",
			url:         "https://wrong-host.com/download/v1/test:download?alt=media",
			shouldError: true,
			errorMsg:    "invalid host",
		},
		{
			name:        "invalid path prefix",
			url:         "https://artifactregistry.googleapis.com/wrong/v1/test:download?alt=media",
			shouldError: true,
			errorMsg:    "invalid path",
		},
		{
			name:        "invalid path suffix",
			url:         "https://artifactregistry.googleapis.com/download/v1/test?alt=media",
			shouldError: true,
			errorMsg:    "must end with ':download'",
		},
		{
			name:        "missing alt=media",
			url:         "https://artifactregistry.googleapis.com/download/v1/test:download",
			shouldError: true,
			errorMsg:    "missing 'alt=media'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := downloadGCloudGenericArtifact(tt.url)
			if !tt.shouldError {
				t.Errorf("Expected no error, got: %v", err)
				return
			}
			if err == nil {
				t.Error("Expected error, got nil")
				return
			}
			if !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing %q, got: %v", tt.errorMsg, err)
			}
		})
	}
}

// Mock test for HTTP download (without actually calling GCloud API)
func TestDownloadGCloudGenericArtifact_HTTPError(t *testing.T) {
	// Create a test server that returns an error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Replace the googleapis.com URL with our test server
	testURL := strings.Replace(
		"https://artifactregistry.googleapis.com/download/v1/projects/test/locations/test/repositories/test/files/test:1.0.0:test.wasm:download?alt=media",
		"https://artifactregistry.googleapis.com",
		server.URL,
		1,
	)

	// This test will fail at the GCloud API call, but we can test URL parsing
	_, err := downloadGCloudGenericArtifact(testURL)
	if err == nil {
		t.Error("Expected error for invalid test URL")
	}
}

// Integration test example (commented out as it requires actual GCloud access)
func TestDownloadWasmModule_Integration(t *testing.T) {
	// Skip this test in CI/CD or when INTEGRATION_TEST env var is not set
	if os.Getenv("INTEGRATION_TEST") != "1" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=1 to run.")
	}

	testURL := "https://artifactregistry.googleapis.com/download/v1/projects/tum-cm/locations/europe-west3/repositories/wasm/files/wasm:1.0.0:3mm_with_cr.wasm:download?alt=media"

	filePath, err := downloadWasmModule(testURL)
	if err != nil {
		t.Fatalf("downloadWasmModule() failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Downloaded file should exist")
	}

	// Clean up
	defer deleteWasmModule(filePath)

	// Verify file has .wasm extension
	if !strings.HasSuffix(filePath, ".wasm") {
		t.Errorf("Downloaded file should have .wasm extension, got: %s", filePath)
	}
}

func TestDownloadLocation(t *testing.T) {
	// Test that download location is accessible
	if downloadLocation == "" {
		t.Error("downloadLocation should not be empty")
	}

	// Check if download location is writable
	testFile := filepath.Join(downloadLocation, "test-write.tmp")
	file, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("Download location %s is not writable: %v", downloadLocation, err)
	}
	file.Close()
	os.Remove(testFile)
}
