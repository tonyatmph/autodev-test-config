//go:build !darwin

package secrets

import (
	"context"
	"fmt"
)

type KeychainProvider struct {
	Service string
}

func (p KeychainProvider) Resolve(context.Context, string) (Value, error) {
	return Value{}, fmt.Errorf("%w: keychain unsupported on this platform", ErrNotFound)
}
