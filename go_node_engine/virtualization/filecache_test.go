package virtualization_test

import (
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/afero"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"

	"go_node_engine/virtualization"
)

func TestNewFileCacheEmptyDir(t *testing.T) {
	fs := afero.NewMemMapFs()
	cacheDir := "/cache"

	cache, err := virtualization.NewFileCache(fs, cacheDir)
	assert.NilError(t, err)

	entries, err := afero.ReadDir(fs, cacheDir)
	assert.NilError(t, err)
	assert.Check(t, cmp.Len(entries, 0))

	// Remove should delete the directory
	assert.NilError(t, cache.Remove())
	exists, err := afero.DirExists(fs, cacheDir)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(false, exists))
}

func TestStoreAndRetrieve(t *testing.T) {
	fs := afero.NewMemMapFs()
	cacheDir := "/cache"
	cache, err := virtualization.NewFileCache(fs, cacheDir)
	assert.NilError(t, err)

	provider := func(key string) (io.Reader, error) {
		return strings.NewReader("hello," + key), nil
	}
	dst := "/destination"

	// first store (cache miss)
	assert.NilError(t, cache.Store("foo", provider, dst))

	// dst should contain data
	b, err := afero.ReadFile(fs, dst)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal("hello,foo", string(b)))

	// exactly one .oakcache file in cacheDir
	files, err := afero.ReadDir(fs, cacheDir)
	assert.NilError(t, err)
	assert.Check(t, cmp.Len(files, 1))
	assert.Check(t, cmp.Regexp(`\.oakcache$`, files[0].Name()))
}

func TestCacheHitDoesNotReinvokeProvider(t *testing.T) {
	fs := afero.NewMemMapFs()
	cacheDir := "/cache"
	cache, err := virtualization.NewFileCache(fs, cacheDir)
	assert.NilError(t, err)

	count := 0
	provider := func(key string) (io.Reader, error) {
		count++
		return strings.NewReader("data"), nil
	}
	dst := "/destination"

	// first store: provider called once
	assert.NilError(t, cache.Store("k", provider, dst))
	assert.Check(t, cmp.Equal(1, count))

	// second store: cache hit, provider not called again
	assert.NilError(t, cache.Store("k", provider, dst))
	assert.Check(t, cmp.Equal(1, count))

	b, err := afero.ReadFile(fs, dst)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal("data", string(b)))
}

func TestMissingUnderlyingFileRecovery(t *testing.T) {
	fs := afero.NewMemMapFs()
	cacheDir := "/cache"
	cache, err := virtualization.NewFileCache(fs, cacheDir)
	assert.NilError(t, err)

	count := 0
	provider := func(key string) (io.Reader, error) {
		count++
		return strings.NewReader("val"), nil
	}
	dst := "/destination"

	// initial store
	assert.NilError(t, cache.Store("xyz", provider, dst))
	assert.Check(t, cmp.Equal(1, count))

	// delete the .oakcache file directly
	files, err := afero.ReadDir(fs, cacheDir)
	assert.NilError(t, err)
	assert.Check(t, cmp.Len(files, 1))
	assert.NilError(t, fs.Remove(filepath.Join(cacheDir, files[0].Name())))

	// second store: provider called again
	assert.NilError(t, cache.Store("xyz", provider, dst))
	assert.Check(t, cmp.Equal(2, count))
}

func TestRemove(t *testing.T) {
	fs := afero.NewMemMapFs()
	cacheDir := "/cache"
	cache, err := virtualization.NewFileCache(fs, cacheDir)
	assert.NilError(t, err)

	provider := func(key string) (io.Reader, error) {
		return strings.NewReader("x"), nil
	}
	dst := "/destination"

	assert.NilError(t, cache.Store("a", provider, dst))
	assert.NilError(t, cache.Remove())

	exists, err := afero.DirExists(fs, cacheDir)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(false, exists))

	// further Store is undefined behaviour
}

func TestConcurrentStoreDifferentKeys(t *testing.T) {
	fs := afero.NewMemMapFs()
	cacheDir := "/cache"
	cache, err := virtualization.NewFileCache(fs, cacheDir)
	assert.NilError(t, err)

	keys := []string{"k1", "k2", "k3", "k4"}

	// barrier so all providers block until every goroutine arrives
	var barrier sync.WaitGroup
	barrier.Add(len(keys))
	provider := func(key string) (io.Reader, error) {
		barrier.Done()
		barrier.Wait()
		return strings.NewReader(key), nil
	}

	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			dst := "/destination-" + k
			assert.NilError(t, cache.Store(k, provider, dst))
			buf, err := afero.ReadFile(fs, dst)
			assert.NilError(t, err)
			assert.Check(t, cmp.Equal(k, string(buf)))
		}(key)
	}
	wg.Wait()

	// all .oakcache files should be present
	files, err := afero.ReadDir(fs, cacheDir)
	assert.NilError(t, err)
	assert.Check(t, cmp.Len(files, len(keys)))
	for _, f := range files {
		assert.Check(t, cmp.Regexp(`\.oakcache$`, f.Name()))
	}
}
