package iotools

import (
	"encoding/json"
	"os"

	"github.com/spf13/afero"
)

func LoadJSON[T any](jsonPath string) (*T, error) {
	return LoadJSONInFs[T](afero.NewOsFs(), jsonPath)
}

func LoadJSONInFs[T any](fs afero.Fs, jsonPath string) (*T, error) {
	jsonFile, err := fs.OpenFile(jsonPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer CloseOrWarn(jsonFile, jsonPath)

	decoder := json.NewDecoder(jsonFile)

	var value T
	if err = decoder.Decode(&value); err != nil {
		return nil, err
	}

	return &value, nil
}

func StoreJSON[T any](value *T, jsonPath string, perm os.FileMode) error {
	return StoreJSONWithIndent(value, jsonPath, perm, "")
}

func StoreJSONInFs[T any](value *T, fs afero.Fs, jsonPath string, perm os.FileMode) error {
	return StoreJSONWithIndentInFs(value, fs, jsonPath, perm, "")
}

func StoreJSONWithIndent[T any](value *T, jsonPath string, perm os.FileMode, indent string) error {
	return StoreJSONWithIndentInFs(value, afero.NewOsFs(), jsonPath, perm, indent)
}

func StoreJSONWithIndentInFs[T any](value *T, fs afero.Fs, jsonPath string, perm os.FileMode, indent string) error {
	jsonFile, err := fs.OpenFile(jsonPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer CloseOrWarn(jsonFile, jsonPath)

	encoder := json.NewEncoder(jsonFile)
	encoder.SetIndent("", indent)

	return encoder.Encode(&value)
}
