package virtualization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go_node_engine/logger"
	"io"
	"net/http"
	"net/url"
	"os"
	pt "path"
	"strings"

	artifactregistry "cloud.google.com/go/artifactregistry/apiv1"
	artifactregistrypb "cloud.google.com/go/artifactregistry/apiv1/artifactregistrypb"
	"google.golang.org/api/option"
)

const downloadLocation = "/tmp"

// Given the wasm module URL, download the module and return the file path
func downloadWasmModule(artifactUrl string) (string, error) {
	logger.InfoLogger().Printf("Downloading artifact from %s\n", artifactUrl)

	// Select the correct download function based on the artifact URL
	if isGCloudArtifactUrl(artifactUrl) {
		filePath, err := downloadGCloudGenericArtifact(artifactUrl)
		if err != nil {
			return "", err
		}
		logger.InfoLogger().Printf("Artifact downloaded successfully: %s\n", filePath)
		return filePath, nil
	} else {
		logger.ErrorLogger().Printf("Invalid artifact URL: %s\n", artifactUrl)
		return "", fmt.Errorf("invalid artifact URL")
	}
}

// Delete a wasm module file
func deleteWasmModule(filePath string) error {
	err := os.Remove(filePath)
	if err != nil {
		logger.ErrorLogger().Printf("Error deleting file: %v\n", err)
		return err
	}
	logger.InfoLogger().Printf("File deleted successfully: %s\n", filePath)
	return nil
}

// Download a generic artifact from Google Cloud Artifact Registry
func downloadGCloudGenericArtifact(artifactUrl string) (string, error) {
	// Parse and validate the URL
	parsedUrl, err := url.Parse(artifactUrl)
	if err != nil {
		return "", fmt.Errorf("invalid artifact URL: %v", err)
	}

	if !strings.HasPrefix(parsedUrl.Scheme, "http") {
		return "", fmt.Errorf("invalid URL scheme: %s", parsedUrl.Scheme)
	}

	if parsedUrl.Host != "artifactregistry.googleapis.com" {
		return "", fmt.Errorf("invalid host in artifact URL: %s", parsedUrl.Host)
	}

	if !strings.HasPrefix(parsedUrl.Path, "/download/v1/") {
		return "", fmt.Errorf("invalid path in artifact URL: must start with '/download/v1/'")
	}

	if !strings.HasSuffix(parsedUrl.Path, ":download") {
		return "", fmt.Errorf("invalid path in artifact URL: must end with ':download'")
	}

	if !strings.Contains(parsedUrl.RawQuery, "alt=media") {
		return "", fmt.Errorf("invalid query in artifact URL: missing 'alt=media'")
	}

	// Extract the artifact path from the URL
	artifactPath := strings.TrimPrefix(parsedUrl.Path, "/download/v1/")
	artifactPath = strings.TrimSuffix(artifactPath, ":download")
	logger.InfoLogger().Printf("Artifact path: %s\n", artifactPath)

	ext := pt.Ext(artifactPath)

	// Connect to the Google Cloud Artifact Registry
	ctx := context.Background()
	c, err := artifactregistry.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		return "", fmt.Errorf("artifactregistry.NewClient: %v", err)
	}
	defer c.Close()

	// Create the GetFile request with the correct name
	req := &artifactregistrypb.GetFileRequest{
		Name: artifactPath,
	}

	file, err := c.GetFile(ctx, req)
	if err != nil {
		return "", fmt.Errorf("c.GetFile: %v", err)
	}

	hashes := file.GetHashes()
	var sha256Hash string
	for _, hash := range hashes {
		if hash.Type == artifactregistrypb.Hash_SHA256 {
			// Convert the hash to a string using hex encoding
			sha256Hash = hex.EncodeToString(hash.Value)
			break
		}
	}

	if sha256Hash == "" {
		return "", fmt.Errorf("SHA256 hash not found in file metadata")
	}

	logger.InfoLogger().Printf("SHA256 hash: %s\n", sha256Hash)

	filename := fmt.Sprintf("%s%s", sha256Hash, ext)

	// Get the absolute path of the output file
	outputFile := pt.Join(downloadLocation, filename)

	// If the file already exists, return the path
	if _, err := os.Stat(outputFile); err == nil {
		logger.InfoLogger().Printf("File already exists: %s\n", outputFile)
		return outputFile, nil
	}

	// Create the file
	out, err := os.Create(outputFile)
	if err != nil {
		return "", fmt.Errorf("os.Create: %v", err)
	}
	defer out.Close()

	// Send the HTTP GET request
	resp, err := http.Get(artifactUrl)
	if err != nil {
		return "", fmt.Errorf("http.Get: %v", err)
	}
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()

	// Check the response status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP status code: %d", resp.StatusCode)
	}

	// Write the response body to the file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("io.Copy: %v", err)
	}

	// Check the SHA256 hash of the downloaded file
	downloadedSHA256, err := getFileSHA256(outputFile)
	if err != nil {
		return "", fmt.Errorf("getFileSHA256: %v", err)
	}

	// Compare the SHA256 hash of the downloaded file with the expected hash
	if downloadedSHA256 != sha256Hash {
		return "", fmt.Errorf("SHA256 hash mismatch: expected %s, got %s", sha256Hash, downloadedSHA256)
	}

	return outputFile, nil
}

// Calculate the SHA256 hash of a file
func getFileSHA256(filePath string) (string, error) {
	hasher := sha256.New()

	// Open the file
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// Copy the file into the hasher
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}

	// Get the hash sum
	hashInBytes := hasher.Sum(nil)

	// Convert the hash to a string using hex encoding
	sha256Hash := hex.EncodeToString(hashInBytes)

	return sha256Hash, nil
}

// Check if the URL of artifact is a valid GCloud Artifact URL
func isGCloudArtifactUrl(urlStr string) bool {
	return strings.HasPrefix(urlStr, "https://artifactregistry.googleapis.com/")
}
