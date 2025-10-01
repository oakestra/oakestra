package image

import (
	"context"
	"errors"
	"fmt"
	"go_node_engine/logger"
	"go_node_engine/util/iotools"
	"go_node_engine/virtualization/internal/crosvm/internal/fsimg"
	"go_node_engine/virtualization/internal/crosvm/internal/kernel"
	"io"
	"os"
	"path"
	"slices"
	"sync"

	"github.com/containers/image/v5/image"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/pkg/blobinfocache/none"
	"github.com/containers/image/v5/types"
	"github.com/containers/storage/pkg/chrootarchive"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
)

var errUnsupportedLayerType = errors.New("unsupported layer type")

var supportedLayerTypes = []string{
	ociv1.MediaTypeImageLayer,
	ociv1.MediaTypeImageLayerGzip,
	ociv1.MediaTypeImageLayerZstd,
}

type ContainersSource struct {
	transport types.ImageTransport
}

func NewContainersSource(transport types.ImageTransport) *ContainersSource {
	return &ContainersSource{
		transport: transport,
	}
}

func (s *ContainersSource) Name() string {
	return "containers-" + s.transport.Name()
}

// Retrieve retrieves a container image using the containers library and converts it into an Image.
// -
// The referenced container image must contain a kernel (and optionally an initrd), which they usually don't.
// This means that most compatible images will be built specifically for this use case.
//
// Internally, this function extracts the kernel (and initrd) from the root filesystem of the container image
// and then constructs an ext4 filesystem image based on it.
func (s *ContainersSource) Retrieve(id string, dstDirPath string) error {
	ctx := context.Background()
	sys := &types.SystemContext{}

	imageRef, err := s.transport.ParseReference(id)
	if err != nil {
		return err
	}

	imageSource, err := imageRef.NewImageSource(ctx, sys)
	if err != nil {
		return err
	}
	defer iotools.CloseOrWarn(imageSource, "ImageSource "+id)

	_, _, imageLayers, err := chooseImageParts(ctx, sys, imageSource)
	if err != nil {
		return err
	}

	// TODO(axiphi): check a special label in the manifest , so that only container images specifically built for this use-case work

	layersDirRemoved := false // allow removing the layers directory early
	layersDirPath, err := iotools.CreateSubDir(dstDirPath, "layers", 0o700)
	if err != nil {
		logger.ErrorLogger().Printf("failed to create layers directory for %q: %v", id, err)
		return err
	}
	defer func() {
		if !layersDirRemoved {
			iotools.RemoveAllOrWarn(layersDirPath)
		}
	}()

	rootfsDirPath, err := iotools.CreateSubDir(dstDirPath, "rootfs", 0o700)
	if err != nil {
		logger.ErrorLogger().Printf("failed to create rootfs directory for %q: %v", id, err)
		return err
	}
	defer iotools.RemoveAllOrWarn(rootfsDirPath)

	// fetching the layers can happen concurrently
	wg := sync.WaitGroup{}
	errs := make([]error, len(imageLayers))
	for i, layer := range imageLayers {
		if !slices.Contains(supportedLayerTypes, layer.MediaType) {
			errs[i] = fmt.Errorf("failed to handle layer %q of image %q: %w", layer.Digest.String(), id, errUnsupportedLayerType)
			break
		}

		wg.Add(1)
		go func(layer types.BlobInfo, i int) {
			defer wg.Done()

			blobReader, _, err := imageSource.GetBlob(ctx, layer, none.NoCache)
			if err != nil {
				errs[i] = err
				return
			}
			defer iotools.CloseOrWarn(blobReader, "Blob "+layer.Digest.String())

			blobPath := path.Join(layersDirPath, layer.Digest.String())
			blobFile, err := os.OpenFile(blobPath, os.O_WRONLY|os.O_CREATE|os.O_CREATE, 0o600)
			if err != nil {
				errs[i] = err
				return
			}
			defer iotools.CloseOrWarn(blobFile, blobPath)

			if _, err := io.Copy(blobFile, blobReader); err != nil {
				errs[i] = err
				return
			}
		}(layer, i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}

	// applying the layers needs to happen consecutively
	for _, layer := range imageLayers {
		err := func(layer types.BlobInfo) error {
			layerPath := path.Join(layersDirPath, layer.Digest.String())
			layerFile, err := os.OpenFile(layerPath, os.O_RDONLY, 0)
			if err != nil {
				return err
			}
			defer iotools.CloseOrWarn(layerFile, layerPath)

			if _, err := chrootarchive.ApplyLayer(rootfsDirPath, layerFile); err != nil {
				return err
			}

			return nil
		}(layer)
		if err != nil {
			return err
		}
	}

	// remove layers directory early to save disk space
	if err := os.RemoveAll(layersDirPath); err != nil {
		logger.WarnLogger().Printf("failed to remove directory %q", layersDirPath)
	}
	layersDirRemoved = true

	relativeKernelPath, relativeInitrdPath, err := kernel.FindLinuxKernelFiles(rootfsDirPath)
	if err != nil {
		return err
	}

	kernelPath := path.Join(dstDirPath, KernelFileName)
	if err := iotools.CopyFile(relativeKernelPath, kernelPath, 0600); err != nil {
		return err
	}

	initrdPath := ""
	if relativeInitrdPath != "" {
		initrdPath = path.Join(dstDirPath, InitrdFileName)
		if err := iotools.CopyFile(relativeInitrdPath, initrdPath, 0600); err != nil {
			return err
		}
	}

	rootfsPath := path.Join(dstDirPath, internalRootfsFileName)
	if err := fsimg.PackIntoSquashFsImg(rootfsDirPath, rootfsPath); err != nil {
		return err
	}

	return nil
}

func chooseImageParts(
	ctx context.Context,
	sys *types.SystemContext,
	imageSource types.ImageSource,
) (*ociv1.Manifest, *ociv1.Image, []types.BlobInfo, error) {
	topImage := image.UnparsedInstance(imageSource, nil)
	topManifestBytes, topManifestType, err := topImage.Manifest(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	if topManifestType == ociv1.MediaTypeImageManifest {
		chosenImage, err := image.FromUnparsedImage(ctx, sys, topImage)
		if err != nil {
			return nil, nil, nil, err
		}

		chosenManifest, err := manifest.OCI1FromManifest(topManifestBytes)
		if err != nil {
			return nil, nil, nil, err
		}

		chosenConfig, err := chosenImage.OCIConfig(ctx)
		if err != nil {
			return nil, nil, nil, err
		}

		return &chosenManifest.Manifest, chosenConfig, chosenImage.LayerInfos(), nil
	}

	if topManifestType == ociv1.MediaTypeImageIndex {
		imageIndex, err := manifest.OCI1IndexFromManifest(topManifestBytes)
		if err != nil {
			return nil, nil, nil, err
		}

		chosenDigest, err := imageIndex.ChooseInstance(nil)
		if err != nil {
			return nil, nil, nil, err
		}

		// in case of a top-level manifest index, this fetches the manifest applicable for the current hardware
		chosenUnparsedImage := image.UnparsedInstance(imageSource, &chosenDigest)
		chosenManifestBytes, chosenManifestType, err := chosenUnparsedImage.Manifest(ctx)
		if err != nil {
			return nil, nil, nil, err
		}

		if chosenManifestType != ociv1.MediaTypeImageManifest {
			return nil, nil, nil, fmt.Errorf("unsupported manifest type")
		}

		chosenImage, err := image.FromUnparsedImage(ctx, sys, chosenUnparsedImage)
		if err != nil {
			return nil, nil, nil, err
		}

		chosenManifest, err := manifest.OCI1FromManifest(chosenManifestBytes)
		if err != nil {
			return nil, nil, nil, err
		}

		chosenConfig, err := chosenImage.OCIConfig(ctx)
		if err != nil {
			return nil, nil, nil, err
		}

		return &chosenManifest.Manifest, chosenConfig, chosenImage.LayerInfos(), nil
	}

	return nil, nil, nil, fmt.Errorf("unsupported manifest type")
}
