package contracts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func ReadFile(path, schemaName string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := Unmarshal(data, schemaName, path, dest); err != nil {
		return err
	}
	return nil
}

func Unmarshal(data []byte, schemaName, documentName string, dest any) error {
	if schemaName != "" {
		if err := Validate(schemaName, documentName, data); err != nil {
			return err
		}
	}
	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("decode %s: %w", documentName, err)
	}
	return nil
}

func Marshal(schemaName, documentName string, value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", documentName, err)
	}
	data = append(data, '\n')
	if schemaName != "" {
		if err := Validate(schemaName, documentName, data); err != nil {
			return nil, err
		}
	}
	return data, nil
}

func WriteFile(path, schemaName string, value any) error {
	data, err := Marshal(schemaName, path, value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write temp %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
