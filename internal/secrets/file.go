package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type FileProvider struct {
	path   string
	values map[string]string
}

func NewFileProvider(path string) (*FileProvider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string)
	if err := json.Unmarshal(data, &values); err != nil {
		return nil, fmt.Errorf("decode smoke secrets fixture: %w", err)
	}
	return &FileProvider{
		path:   path,
		values: values,
	}, nil
}

func (p *FileProvider) Resolve(_ context.Context, name string) (Value, error) {
	if p == nil {
		return Value{}, ErrNotFound
	}
	value, ok := p.values[name]
	if !ok {
		return Value{}, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return Value{
		Name:   name,
		Source: "fixture:" + p.path,
		Value:  value,
	}, nil
}
