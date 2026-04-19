package runner

import (
	"context"
	"g7.mph.tech/mph-tech/autodev/internal/model"
	"g7.mph.tech/mph-tech/autodev/internal/stagecontainer"
)

// Engine abstracts the execution of a stage.
type Engine interface {
	Run(ctx context.Context, spec model.StageSpec, runID string, image stagecontainer.RuntimeImage) error
}

// DockerEngine implements Engine using stagecontainer.
type DockerEngine struct {
	cfg stagecontainer.Config
}

func NewDockerEngine(cfg stagecontainer.Config) *DockerEngine {
	return &DockerEngine{cfg: cfg}
}

func (d *DockerEngine) Run(ctx context.Context, spec model.StageSpec, runID string, image stagecontainer.RuntimeImage) error {
	runner := &stagecontainer.Docker{}
	return runner.Run(ctx, d.cfg, spec, runID, image)
}
