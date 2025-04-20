package virtualization_test

import (
	"crypto/sha256"
	"encoding/hex"
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

func TestInitializeFromEmptyDir(t *testing.T) {
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

func TestInitializeFromPrefilledDir(t *testing.T) {
	fs := afero.NewMemMapFs()
	cacheDir := "/cache"

	// Prepare the cache dir
	assert.NilError(t, fs.MkdirAll(cacheDir, 0o750))

	// Preload a valid .oakcache file for key "foo"
	key := "foo"
	sum := sha256.Sum256([]byte(key))
	filename := hex.EncodeToString(sum[:]) + ".oakcache"
	fullPath := filepath.Join(cacheDir, filename)
	assert.NilError(t, afero.WriteFile(fs, fullPath, []byte("preloaded"), 0o644))

	// Add an extraneous file and a subdirectory
	assert.NilError(t, afero.WriteFile(fs, filepath.Join(cacheDir, "ignore.txt"), []byte("x"), 0o644))
	assert.NilError(t, fs.MkdirAll(filepath.Join(cacheDir, "subdir"), 0o755))

	// Initialize cache
	cache, err := virtualization.NewFileCache(fs, cacheDir)
	assert.NilError(t, err)

	// Directory should still contain three entries: <hash>.oakcache, ignore.txt, subdir
	entries, err := afero.ReadDir(fs, cacheDir)
	assert.NilError(t, err)
	assert.Check(t, cmp.Len(entries, 3))

	// Save for "foo" should be a cache hit
	count := 0
	provider := func(k string) (io.Reader, error) {
		count++
		return strings.NewReader("should-not-be-used"), nil
	}
	dst := "/dest"

	assert.NilError(t, cache.Save(key, provider, dst))
	// provider not called
	assert.Check(t, cmp.Equal(count, 0))

	// dst should contain the preloaded data
	buf, err := afero.ReadFile(fs, dst)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal("preloaded", string(buf)))
}

func TestRemoveOnlyDeletesCacheFiles(t *testing.T) {
	fs := afero.NewMemMapFs()
	cacheDir := "/cache"

	// Preload one .oakcache file
	key := "foo"
	sum := sha256.Sum256([]byte(key))
	fname := hex.EncodeToString(sum[:]) + ".oakcache"
	cachePath := filepath.Join(cacheDir, fname)
	assert.NilError(t, afero.WriteFile(fs, cachePath, []byte("data"), 0o644))

	// Add an extra file and a subdirectory that should be preserved
	extraFile := filepath.Join(cacheDir, "keep.txt")
	assert.NilError(t, afero.WriteFile(fs, extraFile, []byte("x"), 0o644))
	extraDir := filepath.Join(cacheDir, "subdir")
	assert.NilError(t, fs.MkdirAll(extraDir, 0o755))

	// Create cache
	cache, err := virtualization.NewFileCache(fs, cacheDir)
	assert.NilError(t, err)

	// Sanity: all three entries exist
	entries, err := afero.ReadDir(fs, cacheDir)
	assert.NilError(t, err)
	assert.Check(t, cmp.Len(entries, 3))

	// Remove should delete only the .oakcache file and leave extras
	assert.NilError(t, cache.Remove())

	// Directory must still exist (because extras were found)
	exists, err := afero.DirExists(fs, cacheDir)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(true, exists))

	// List remaining entries
	remaining, err := afero.ReadDir(fs, cacheDir)
	assert.NilError(t, err)
	// Expect only the extra file and subdir
	assert.Check(t, cmp.Len(remaining, 2))
	names := map[string]bool{}
	for _, e := range remaining {
		names[e.Name()] = true
	}
	assert.Check(t, cmp.DeepEqual(
		names,
		map[string]bool{"keep.txt": true, "subdir": true},
	))
}

func TestSaveAndRetrieve(t *testing.T) {
	fs := afero.NewMemMapFs()
	cacheDir := "/cache"
	cache, err := virtualization.NewFileCache(fs, cacheDir)
	assert.NilError(t, err)

	provider := func(key string) (io.Reader, error) {
		return strings.NewReader("hello," + key), nil
	}
	dst := "/destination"

	// first save (cache miss)
	assert.NilError(t, cache.Save("foo", provider, dst))

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

	// first save: provider called once
	assert.NilError(t, cache.Save("k", provider, dst))
	assert.Check(t, cmp.Equal(1, count))

	// second save: cache hit, provider not called again
	assert.NilError(t, cache.Save("k", provider, dst))
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

	// initial save
	assert.NilError(t, cache.Save("xyz", provider, dst))
	assert.Check(t, cmp.Equal(1, count))

	// delete the .oakcache file directly
	files, err := afero.ReadDir(fs, cacheDir)
	assert.NilError(t, err)
	assert.Check(t, cmp.Len(files, 1))
	assert.NilError(t, fs.Remove(filepath.Join(cacheDir, files[0].Name())))

	// second save: provider called again
	assert.NilError(t, cache.Save("xyz", provider, dst))
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

	assert.NilError(t, cache.Save("a", provider, dst))
	assert.NilError(t, cache.Remove())

	exists, err := afero.DirExists(fs, cacheDir)
	assert.NilError(t, err)
	assert.Check(t, cmp.Equal(false, exists))

	// further Save is undefined behaviour
}

func TestConcurrentSaveDifferentKeys(t *testing.T) {
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
			assert.NilError(t, cache.Save(k, provider, dst))
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
