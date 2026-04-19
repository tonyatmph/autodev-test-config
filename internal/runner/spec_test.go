package runner

import (
	"fmt"
	"os"
	"strings"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

func orchestratedSpec(spec model.StageSpec, module string) model.StageSpec {
	stageName := module
	if idx := strings.LastIndex(module, "."); idx >= 0 && idx < len(module)-1 {
		stageName = module[idx+1:]
	}
	spec.Operation = "orchestrate"
	spec.OperationConfig = map[string]any{
		"steps": []map[string]any{
			{
				"name":    stageName,
				"command": []string{"/bin/true"},
			},
		},
	}
	if spec.Runtime.QueueMode == "" {
		spec.Runtime.QueueMode = model.StageQueueModeAuto
	}
	if spec.Container.Permissions.RuntimeUser.Mode == "" {
		spec.Container.Permissions.RuntimeUser = model.RuntimeUserSpec{
			Mode:          model.RuntimeIsolationModeContainer,
			ContainerUser: fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		}
	}
	spec.Entrypoint = []string{"autodev-stage-runtime"}
	return spec
}
