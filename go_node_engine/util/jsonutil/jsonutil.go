package jsonutil

import (
	"encoding/json"
	"github.com/spf13/afero"
	"go_node_engine/logger"
	"os"
)

func LoadJSON[T any](jsonPath string) (*T, error) {
	return LoadJSONInFs[T](afero.NewOsFs(), jsonPath)
}

func LoadJSONInFs[T any](fs afero.Fs, jsonPath string) (*T, error) {
	jsonFile, err := fs.OpenFile(jsonPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := jsonFile.Close(); err != nil {
			logger.WarnLogger().Printf("failed to close JSON file %q: %v", jsonPath, err)
		}
	}()

	decoder := json.NewDecoder(jsonFile)

	var value T
	if err = decoder.Decode(&value); err != nil {
		return nil, err
	}

	return &value, nil
}

func StoreJSON[T any](value *T, jsonPath string, perm os.FileMode) error {
	return StoreJSONInFs(value, afero.NewOsFs(), jsonPath, perm)
}

func StoreJSONInFs[T any](value *T, fs afero.Fs, jsonPath string, perm os.FileMode) error {
	jsonFile, err := fs.OpenFile(jsonPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer func() {
		if err := jsonFile.Close(); err != nil {
			logger.WarnLogger().Printf("failed to close JSON file %q: %v", jsonPath, err)
		}
	}()

	encoder := json.NewEncoder(jsonFile)

	return encoder.Encode(&value)
}
