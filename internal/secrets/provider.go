package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrNotFound = errors.New("secret not found")

type Provider interface {
	Resolve(ctx context.Context, name string) (Value, error)
}

type Value struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Value  string `json:"-"`
}

type Chain struct {
	providers []Provider
}

func NewChain(providers ...Provider) *Chain {
	usable := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			usable = append(usable, provider)
		}
	}
	return &Chain{providers: usable}
}

func (c *Chain) Resolve(ctx context.Context, name string) (Value, error) {
	var errs []string
	for _, provider := range c.providers {
		value, err := provider.Resolve(ctx, name)
		if err == nil {
			return value, nil
		}
		if errors.Is(err, ErrNotFound) {
			continue
		}
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return Value{}, errors.New(strings.Join(errs, "; "))
	}
	return Value{}, fmt.Errorf("%w: %s", ErrNotFound, name)
}

func ResolveAll(ctx context.Context, provider Provider, names []string) ([]Value, error) {
	values := make([]Value, 0, len(names))
	for _, name := range names {
		value, err := provider.Resolve(ctx, name)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}
