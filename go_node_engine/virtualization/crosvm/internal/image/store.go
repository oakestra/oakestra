package image

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go_node_engine/util/dirutil"
	"go_node_engine/util/fileutil"
	"go_node_engine/virtualization/crosvm/internal/fsimg"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"go_node_engine/logger"
)

var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)

const (
	KernelFileName     = "kernel.img"
	InitrdFileName     = "initrd.img"
	RootfsFileName     = "rootfs.img"
	RootfsMountDirName = "rootfs-mount"

	imageDirExtension      = ".oakimg"
	internalRootfsFileName = "rootfs.squashfs"
)

// Store retrieves and caches the files and information needed to run crosvm instances.
// The Retrieve method is safe and intended for concurrent use, but users of Store need to ensure the Remove method
// is only called once and only after all uses of Retrieve have completed.
//
// NOTE: Store does not use afero.Fs, because it's implemented using the external mksquashfs and unsquashfs commands,
// which only work on the OS filesystem.
type Store struct {
	dirPath       string
	defaultSource Source
	sources       map[string]Source
	mutex         sync.RWMutex
	images        map[string]*Image
	totalSize     int64
}

// NewStore ensures dirPath exists, scans it for existing “*.oakcache” directories, and indexes them.
func NewStore(dirPath string, sources ...Source) (*Store, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("store needs to have atleast one source")

	}

	sourcesMap := make(map[string]Source)
	for _, source := range sources {
		sourcesMap[source.Name()] = source
	}
	defaultSource := sources[0]

	if err := os.Mkdir(dirPath, 0o700); err != nil && !os.IsExist(err) {
		logger.ErrorLogger().Printf("failed to create store directory %q: %v", dirPath, err)
		return nil, err
	}

	logger.InfoLogger().Printf("initializing store at %q", dirPath)
	images, err := restoreImages(dirPath)
	if err != nil {
		logger.ErrorLogger().Printf("failed to restore images: %v", err)
		return nil, err
	}

	imagesMap := make(map[string]*Image)
	var totalSize int64 = 0
	for _, img := range images {
		imagesMap[img.Key] = &img
		totalSize += img.Size
	}

	logger.InfoLogger().Printf("restored %d images, total %d bytes", len(images), totalSize)

	p := &Store{
		dirPath:       dirPath,
		defaultSource: defaultSource,
		sources:       sourcesMap,

		mutex:     sync.RWMutex{},
		images:    imagesMap,
		totalSize: totalSize,
	}

	return p, nil
}

// Retrieve ensures the image contents specified by ref are cached under its top-level digest, then copies them to dst.
// Multiple distinct keys may fetch in parallel.
//
// When Retrieve is concurrently called multiple times for the same, uncached image,
// the Store might try to retrieve the corresponding src multiple times,
// in which case all of them but the first one to complete is thrown away.
func (s *Store) Retrieve(ref string, dstDirPath string, rootfsSize int64) (*Image, error) {
	source, id, err := s.parseRef(ref)
	if err != nil {
		return nil, err
	}

	key := convertIdToKey(source, id)
	imageDirPath := filepath.Join(s.dirPath, key+imageDirExtension)

	foundInCache, img, err := s.retrieveFromCache(ref, key, imageDirPath, dstDirPath, rootfsSize)
	if foundInCache {
		return img, err
	}

	tmpDirPath, err := dirutil.CreateLargeTempDir("image")
	if err != nil {
		logger.ErrorLogger().Printf("failed to create temporary directory for %q: %v", ref, err)
		return nil, err
	}

	if err = source.Retrieve(id, tmpDirPath); err != nil {
		dirutil.RemoveAllOrWarn(tmpDirPath)
		return nil, err
	}

	img, err = CreateImageFromDir(tmpDirPath)
	if err != nil {
		dirutil.RemoveAllOrWarn(tmpDirPath)
		return nil, err
	}
	if err := copyImageFilesToDst(tmpDirPath, dstDirPath, rootfsSize, img.HasInitrd); err != nil {
		dirutil.RemoveAllOrWarn(tmpDirPath)
		return nil, err
	}

	go s.moveIntoCache(ref, img, tmpDirPath, imageDirPath)

	return img, nil
}

// Remove deletes every file of the cache and the cache directory itself.
// After Remove returns, the Store cannot not be used anymore and calls on it will result in undefined behavior.
func (s *Store) Remove() error {
	logger.InfoLogger().Printf("removing cache at %q", s.dirPath)

	imageEntries, err := os.ReadDir(s.dirPath)
	if err != nil {
		logger.ErrorLogger().Printf("failed to read directory %q: %v", s.dirPath, err)
		return err
	}

	skippedEntry := false
	for _, imageEntry := range imageEntries {
		imageDirName := imageEntry.Name()
		imageDirPath := filepath.Join(s.dirPath, imageDirName)

		if !isImageDir(imageDirPath) {
			logger.WarnLogger().Printf("while removing store, found non-image directory %q, ignoring", imageDirPath)
			skippedEntry = true
			continue
		}

		if err := os.Remove(imageDirPath); err != nil {
			logger.WarnLogger().Printf("while removing store, failed to remove image directory %q: %v", imageDirPath, err)
			skippedEntry = true
			continue
		}
	}

	if !skippedEntry {
		if err := os.Remove(s.dirPath); err != nil {
			logger.ErrorLogger().Printf("failed to remove store directory %q: %v", s.dirPath, err)
			return err
		}
	}

	return nil
}

// retrieveFromCache is the fast‑path for image retrieval in a Store it, returns (found, image, err)
func (s *Store) retrieveFromCache(
	ref string,
	key string,
	imageDirPath string,
	dstDirPath string,
	rootfsSize int64,
) (bool, *Image, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	img, ok := s.images[key]
	if !ok {
		return false, nil, nil
	}

	if err := copyImageFilesToDst(imageDirPath, dstDirPath, rootfsSize, img.HasInitrd); err != nil {
		// allow the cache to recover if a file of an entry was deleted, by pretending it wasn't cached
		if os.IsNotExist(err) {
			logger.WarnLogger().Printf("missing file for cached entry %q, restoring: %v", ref, err)
			return false, nil, nil
		}
		return true, nil, err
	}

	logger.InfoLogger().Printf("cache hit for %q -> %q", ref, imageDirPath)

	return true, img, nil
}

// moveIntoCache moves a temporary file with the contents for the specified key into the cache and then copies it to dst
func (s *Store) moveIntoCache(ref string, img *Image, tmpDirPath string, imageDirPath string) {
	// move the temporary file to the cache directory, if no other concurrent accessor already did so
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// The image might already in the cache at this point, this could have two reasons:
	// - another concurrent call of this function retrieved the image at the same time and put it in the cache before us
	// - the image was present in the cache, but some underlying file was missing, so we are currently re-retrieving it
	// By just replacing the already existing image directory we get the correct behavior no matter what.

	_ = os.RemoveAll(imageDirPath)

	if err := os.Rename(tmpDirPath, imageDirPath); err != nil {
		logger.ErrorLogger().Printf("moving image files into cache directory %q failed for image %q, it will be re-fetched the next time it is used: %v", imageDirPath, ref, err)
		dirutil.RemoveAllOrWarn(tmpDirPath)
		return
	}

	if oldImg, ok := s.images[img.Key]; ok {
		s.totalSize -= oldImg.Size
	}

	s.images[img.Key] = img
	s.totalSize += img.Size

	logger.InfoLogger().Printf("moved image %q into cache at %q, total size is now %d bytes", ref, imageDirPath, s.totalSize)
}

// parseRef takes an image reference and splits it into its source and id parts
func (s *Store) parseRef(ref string) (Source, string, error) {
	splitIdx := strings.Index(ref, ":")
	if splitIdx == -1 {
		return s.defaultSource, ref, nil
	}

	sourceName, id := ref[:splitIdx], ref[splitIdx+1:]
	source, ok := s.sources[sourceName]
	if !ok {
		return s.defaultSource, ref, nil
	}

	return source, id, nil
}

func copyImageFilesToDst(
	srcDirPath string,
	dstDirPath string,
	rootfsSize int64,
	hasInitrd bool,
) error {
	if _, err := os.Stat(srcDirPath); err != nil {
		return err
	}

	srcRootfsPath := path.Join(srcDirPath, internalRootfsFileName)
	dstRootfsPath := path.Join(dstDirPath, RootfsFileName)
	rootfsMountDirPath := path.Join(dstDirPath, RootfsMountDirName)
	if err := fsimg.CreateExt4Img(dstRootfsPath, 0o600, rootfsSize); err != nil {
		return err
	}
	if err := fsimg.CopySquashFsIntoExt4Img(srcRootfsPath, dstRootfsPath, rootfsMountDirPath); err != nil {
		return err
	}

	srcKernelPath := path.Join(srcDirPath, KernelFileName)
	dstKernelPath := path.Join(dstDirPath, KernelFileName)
	if err := fileutil.CopyFile(srcKernelPath, dstKernelPath, 0o0700); err != nil {
		return err
	}

	if hasInitrd {
		srcInitrdPath := path.Join(srcDirPath, InitrdFileName)
		dstInitrdPath := path.Join(dstDirPath, InitrdFileName)
		if err := fileutil.CopyFile(srcInitrdPath, dstInitrdPath, 0o0700); err != nil {
			return err
		}
	}

	return nil
}

func convertIdToKey(source Source, id string) string {
	sum := sha256.Sum256([]byte(id))
	hash := hex.EncodeToString(sum[:])
	safeId := nonAlphanumeric.ReplaceAllString(hash, "_")
	return fmt.Sprintf("%s-%s-%s", source.Name(), safeId, hash)
}

func restoreImages(dirPath string) ([]Image, error) {
	imageEntries, err := os.ReadDir(dirPath)
	if err != nil {
		logger.ErrorLogger().Printf("failed to read store directory %q: %v", dirPath, err)
		return nil, err
	}

	images := []Image{}
	for _, imageEntry := range imageEntries {
		imageDirName := imageEntry.Name()
		imageDirPath := filepath.Join(dirPath, imageDirName)

		if !isImageDir(imageDirPath) {
			logger.WarnLogger().Printf("while initializing store, found non-image directory %q, ignoring", imageDirPath)
			continue
		}

		img, err := CreateImageFromDir(imageDirPath)
		if err != nil {
			if err := os.RemoveAll(imageDirPath); err != nil {
				return nil, fmt.Errorf("failed to remove faulty image directory %q: %w", imageDirPath, err)
			}
			continue
		}

		images = append(images, *img)
	}

	return images, nil
}

func isImageDir(imageDirPath string) bool {
	if !strings.HasSuffix(imageDirPath, imageDirExtension) {
		return false
	}

	info, err := os.Stat(imageDirPath)
	if err != nil {
		return false
	}

	if !info.Mode().IsDir() {
		return false
	}

	return true
}
