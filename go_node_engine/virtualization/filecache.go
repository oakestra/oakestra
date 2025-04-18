package virtualization

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go_node_engine/logger"
)

const (
	cacheFileExtension = ".oakcache"
)

// FileCache retrieves and caches files on disk by a caller specified key; safe for concurrent use.
type FileCache struct {
	dirPath   string
	mutex     sync.RWMutex
	fileSizes map[string]int64 // map-key: hex(SHA256(key)), map-value: size
	totalSize int64
}

type ReaderProvider func(key string) (io.Reader, error)

// NewFileCache ensures dirPath exists, scans it for existing files,
// indexes them, and honors removeOnClose on Close().
func NewFileCache(dirPath string) (*FileCache, error) {
	if err := os.MkdirAll(dirPath, 0o750); err != nil {
		logger.ErrorLogger().Printf(
			"FileCache: failed to create cache dirPath %q: %v", dirPath, err,
		)
		return nil, err
	}

	existingEntries, err := os.ReadDir(dirPath)
	if err != nil {
		logger.ErrorLogger().Printf("FileCache: failed to read cache dirPath %q: %v", dirPath, err)
		return nil, err
	}

	p := &FileCache{
		dirPath:   dirPath,
		mutex:     sync.RWMutex{},
		fileSizes: make(map[string]int64),
		totalSize: 0,
	}

	logger.InfoLogger().Printf("FileCache: initializing at %q", dirPath)
	for _, existingEntry := range existingEntries {
		entryName := existingEntry.Name()
		entryPath := filepath.Join(dirPath, entryName)

		info, err := os.Stat(entryPath)
		if err != nil {
			logger.WarnLogger().Printf("FileCache: cannot stat %q, ignoring: %v", entryPath, err)
			continue
		}

		if !info.Mode().IsRegular() {
			logger.WarnLogger().Printf("FileCache: non-file entry %q, ignoring", entryPath)
			continue
		}

		if !strings.HasSuffix(entryName, cacheFileExtension) {
			logger.WarnLogger().Printf("FileCache: unknown entry %q, ignoring", entryPath)
			continue
		}

		p.fileSizes[entryName] = info.Size()
		p.totalSize += info.Size()
	}

	logger.InfoLogger().Printf("FileCache: loaded %d files, total %d bytes", len(p.fileSizes), p.totalSize)
	return p, nil
}

// Store ensures the contents provided by srcProvider are cached under the specified key, then copies them to dst.
func (p *FileCache) Store(key string, srcProvider ReaderProvider, dst string) error {
	// compute hash and final path
	keySum := sha256.Sum256([]byte(key))
	keyHash := hex.EncodeToString(keySum[:])
	fullPath := filepath.Join(p.dirPath, keyHash+cacheFileExtension)

	foundInCache, err := p.copyFromCacheToDst(key, keyHash, fullPath, dst)
	if foundInCache {
		return err
	}

	tmpPath, err := p.retrieveIntoTmpFile(key, srcProvider)
	if err != nil {
		return err
	}

	return p.moveIntoCacheAndCopyToDst(key, keyHash, fullPath, dst, tmpPath)
}

// try fast‑path cache hit under RLock
func (p *FileCache) copyFromCacheToDst(key string, keyHash string, fullPath string, dst string) (bool, error) {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	if _, found := p.fileSizes[keyHash]; !found {
		return false, nil
	}

	// if we'd just call copyFile here and propagate its error to the caller,
	// the cache could never recover from an underlying file going missing
	_, err := os.Stat(fullPath)
	if errors.Is(err, os.ErrNotExist) {
		logger.WarnLogger().Printf("FileCache: missing underlying file for cached entry %q, restoring", fullPath)
		// because of us returning "not found in cache" in the first return value here,
		// the calling function will try to retrieve the underlying file again
		return false, nil
	}

	if err != nil {
		// everything that's not an ErrNotExist error we treat as non-recoverable
		return true, err
	}

	logger.InfoLogger().Printf("FileCache: cache hit for %q -> %q", key, fullPath)
	return true, p.copyFile(fullPath, dst)
}

// moveIntoCacheAndCopyToDst moves a temporary file with the contents for the specified key into the cache and then copies it to dst
func (p *FileCache) moveIntoCacheAndCopyToDst(key string, keyHash string, fullPath string, dst string, tmpPath string) error {
	// get the size of the retrieved file
	info, err := os.Stat(tmpPath)
	if err != nil {
		logger.ErrorLogger().Printf("FileCache: failed to stat temporary file %q for %q: %v", tmpPath, key, err)
		if err := os.Remove(tmpPath); err != nil {
			logger.WarnLogger().Printf("FileCache: failed to remove temporary file %q: %v", tmpPath, err)
		}
		return err
	}
	size := info.Size()

	// move the temporary file to the cache directory, if no other concurrent
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if _, found := p.fileSizes[keyHash]; found {
		// the file with the specified key is already in the cache, this could have two reasons:
		// - another concurrent call of this function just retrieved the file
		// - the key was present in the map, but the underlying file was missing, so we are currently re-retrieving it

		// the condition below is true if the key was present in the map, but the underlying file was missing
		if _, err := os.Stat(fullPath); errors.Is(err, fs.ErrNotExist) {
			// after moving the temporary file to its destination in the cache, we don't need to change the map,
			// since the entry is already present
			if err := os.Rename(tmpPath, fullPath); err != nil {
				logger.ErrorLogger().Printf("FileCache: for %q, moving temporary file %q to destination %q failed: %v", key, tmpPath, fullPath, err)
				if err := os.Remove(tmpPath); err != nil {
					logger.WarnLogger().Printf("FileCache: failed to remove temporary file %q: %v", tmpPath, err)
				}
				return err
			}
			logger.InfoLogger().Printf("FileCache: restored missing file for %q -> %q", key, fullPath)
			err := p.copyFile(fullPath, dst)
			return err
		}

		// if we got to here, another call of this function retrieved the file concurrently
		if err := os.Remove(tmpPath); err != nil {
			logger.WarnLogger().Printf("FileCache: failed to remove temporary file %q: %v", tmpPath, err)
		}
		logger.InfoLogger().Printf("FileCache: using cache after retrieval race for %q -> %q", key, fullPath)
		// we won't try to recover from a missing file here, since we already do that on an initial cache hit
		return p.copyFile(fullPath, dst)
	}

	if err := os.Rename(tmpPath, fullPath); err != nil {
		logger.ErrorLogger().Printf("FileCache: for %q, moving temporary file %q to destination %q failed: %v", key, tmpPath, fullPath, err)
		if err := os.Remove(tmpPath); err != nil {
			logger.WarnLogger().Printf("FileCache: failed to remove temporary file %q: %v", tmpPath, err)
		}
		return err
	}
	p.fileSizes[keyHash] = size
	p.totalSize += size
	logger.InfoLogger().Printf("FileCache: for %q, retrieved to %q (%d bytes), total %d bytes", key, fullPath, size, p.totalSize)
	return p.copyFile(fullPath, dst)
}

// retrieveIntoTmpFile is used after a cache miss and retrieves the contents from srcProvider into a temporary file with a random name
func (p *FileCache) retrieveIntoTmpFile(key string, srcProvider ReaderProvider) (string, error) {
	// after cache miss: create temporary file with random name for retrieval
	tmpFile, err := os.CreateTemp("", "oakestra-filecache-*.tmp")
	if err != nil {
		logger.ErrorLogger().Printf("FileCache: failed to create temporary file for %q: %v", key, err)
		return "", err
	}
	tmpPath := tmpFile.Name()

	logger.InfoLogger().Printf("FileCache: retrieving %q to %q", key, tmpPath)

	src, err := srcProvider(key)
	if err != nil {
		logger.ErrorLogger().Printf("FileCache: failed to retrieve %q: %v", key, err)
		if err := tmpFile.Close(); err != nil {
			logger.WarnLogger().Printf("FileCache: failed to close temporary file %q: %v", tmpPath, err)
		}
		if err := os.Remove(tmpPath); err != nil {
			logger.WarnLogger().Printf("FileCache: failed to remove temporary file %q: %v", tmpPath, err)
		}
		return "", err
	}
	if _, err = io.Copy(tmpFile, src); err != nil {
		logger.ErrorLogger().Printf("FileCache: failed to write to temporary file %q for %q: %v", tmpPath, key, err)
		if err := tmpFile.Close(); err != nil {
			logger.WarnLogger().Printf("FileCache: failed to close temporary file %q: %v", tmpPath, err)
		}
		if err := os.Remove(tmpPath); err != nil {
			logger.WarnLogger().Printf("FileCache: failed to remove temporary file %q: %v", tmpPath, err)
		}
		return "", err
	}

	if err := tmpFile.Close(); err != nil {
		logger.WarnLogger().Printf("FileCache: failed to close temporary file %q: %v", tmpPath, err)
	}
	return tmpPath, nil
}

// copyFile copies from src into dst, creating or truncating the latter. It must be called with the cache’s RWLock held
func (p *FileCache) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		logger.ErrorLogger().Printf("FileCache: failed to open src file %q: %v", src, err)
		return err
	}
	defer func() {
		if err := in.Close(); err != nil {
			logger.WarnLogger().Printf("FileCache: failed to close src file %q: %v", src, err)
		}
	}()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
	if err != nil {
		logger.ErrorLogger().Printf("FileCache: failed to create dst file %q: %v", dst, err)
		return err
	}
	defer func() {
		if err := out.Close(); err != nil {
			logger.WarnLogger().Printf("FileCache: failed to close dst file %q: %v", src, err)
		}
	}()

	logger.InfoLogger().Printf("FileCache: copying %q → %q", src, dst)
	if _, err = io.Copy(out, in); err != nil {
		logger.ErrorLogger().Printf("FileCache: failed to copy from src file %q to dst file %q: %v", src, dst, err)
		return err
	}

	return nil
}

// Remove deletes every file of the cache and the cache directory itself.
// After Remove returns, the FileCache cannot not be used anymore and calls on it will result in undefined behavior.
func (p *FileCache) Remove() error {
	logger.InfoLogger().Printf("FileCache: removing cache at %q", p.dirPath)

	entries, err := os.ReadDir(p.dirPath)
	if err != nil {
		logger.ErrorLogger().Printf("FileCache: failed to read directory %q: %v", p.dirPath, err)
		return err
	}

	skippedFile := false
	for _, existingEntry := range entries {
		entryName := existingEntry.Name()
		entryPath := filepath.Join(p.dirPath, entryName)

		info, err := os.Stat(entryPath)
		if err != nil {
			logger.WarnLogger().Printf("FileCache: while removing cache, cannot stat %q, ignoring: %v", entryPath, err)
			skippedFile = true
			continue
		}

		if !info.Mode().IsRegular() {
			logger.WarnLogger().Printf("FileCache: while removing cache, non-file entry %q, ignoring", entryPath)
			skippedFile = true
			continue
		}

		if !strings.HasSuffix(entryName, cacheFileExtension) {
			logger.WarnLogger().Printf("FileCache: while removing cache, unknown entry %q, ignoring", entryPath)
			skippedFile = true
			continue
		}

		if err := os.Remove(entryPath); err != nil {
			logger.WarnLogger().Printf("FileCache: while removing cache, failed to remove file %q: %v", entryPath, err)
		}
	}

	if !skippedFile {
		if err := os.Remove(p.dirPath); err != nil {
			logger.ErrorLogger().Printf("FileCache: failed to remove cache directory %q: %v", p.dirPath, err)
			return err
		}
	}

	return nil
}
