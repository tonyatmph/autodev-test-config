package runner

import (
	"context"
)

// NativeProvider implements the Provider interface using a local Go function (in-process).
type NativeProvider struct {
	spec     Contract
	executor func(ctx context.Context, inputs []SHA) (SHA, float64, map[string]float64, error)
}

func NewNativeProvider(spec Contract, exec func(ctx context.Context, inputs []SHA) (SHA, float64, map[string]float64, error)) *NativeProvider {
	return &NativeProvider{
		spec:     spec,
		executor: exec,
	}
}

// Dependencies returns the sub-goals required before this provider can execute.
func (p *NativeProvider) Dependencies(ctx context.Context, input SHA) ([]Goal, error) {
	// Native providers can define dependencies if needed.
	return nil, nil
}

// Execute performs the state transition in-process.
func (p *NativeProvider) Execute(ctx context.Context, inputs []SHA, stream TelemetryStream) (SHA, float64, map[string]float64, error) {
	return p.executor(ctx, inputs)
}

