package download

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/oakestra/oakestra/installer/util/iotools"
	"github.com/spf13/afero"
)

const (
	defaultFileTimeout = 10 * time.Minute
	defaultBodyTimeout = 30 * time.Second
)

func withDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func getResponse(ctx context.Context, url string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		defer func() { _ = response.Body.Close() }()
		body, _ := io.ReadAll(io.LimitReader(response.Body, 10*1024))
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		preview := strings.TrimSpace(string(body))
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		if preview != "" {
			return nil, fmt.Errorf("GET %s returned %s: %q", url, response.Status, preview)
		}
		return nil, fmt.Errorf("GET %s returned %s", url, response.Status)
	}
	return response, nil
}

func GetFile(ctx context.Context, url, dstPath string, perm os.FileMode) error {
	return GetFileInFs(ctx, afero.NewOsFs(), url, dstPath, perm)
}

func GetFileInFs(ctx context.Context, fs afero.Fs, url, dstPath string, perm os.FileMode) error {
	ctx, cancel := withDefaultTimeout(ctx, defaultFileTimeout)
	defer cancel()

	response, err := getResponse(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	return iotools.WriteFileAtomicInFs(ctx, fs, dstPath, perm, response.Body)
}

func GetBytes(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := withDefaultTimeout(ctx, defaultBodyTimeout)
	defer cancel()

	response, err := getResponse(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	return io.ReadAll(response.Body)
}

func GetText(ctx context.Context, url string) (string, error) {
	data, err := GetBytes(ctx, url)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
