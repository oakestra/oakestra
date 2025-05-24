package cloudinit

import (
	"encoding/json"
	"go_node_engine/util/iotools"
	"go_node_engine/virtualization/internal/crosvm/internal/fsimg"
	"os"
	"path"
)

func CreateNoCloudFsImg(userData UserData, metaData MetaData, networkConfig NetworkConfig, dstPath string) error {
	tmpDirPath, err := iotools.CreateTempDir("cloud-init")
	if err != nil {
		return err
	}
	defer iotools.RemoveOrWarn(tmpDirPath)

	// technically all files below are YAML files, but since YAML is a JSON superset, we can just use JSON

	// for user-data we can't use the normal StoreJSONWithIndent functionality,
	// since cloud-init requires a comment at the top of the file
	if err := storeUserData(&userData, path.Join(tmpDirPath, "user-data")); err != nil {
		return err
	}

	if err := iotools.StoreJSONWithIndent(&metaData, path.Join(tmpDirPath, "meta-data"), 0o600, "  "); err != nil {
		return err
	}

	if err := iotools.StoreJSONWithIndent(&networkConfig, path.Join(tmpDirPath, "network-config"), 0o600, "  "); err != nil {
		return err
	}

	return fsimg.PackIntoIsoFsImg("CIDATA", tmpDirPath, dstPath)
}

func storeUserData(userData *UserData, dstPath string) error {
	jsonFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer iotools.CloseOrWarn(jsonFile, dstPath)

	if _, err := jsonFile.WriteString("#cloud-config\n"); err != nil {
		return err
	}

	encoder := json.NewEncoder(jsonFile)
	encoder.SetIndent("", "  ")

	return encoder.Encode(&userData)
}
