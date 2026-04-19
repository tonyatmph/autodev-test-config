package stagecontainer

import (
	"fmt"
	"os/exec"
	"strings"

	"g7.mph.tech/mph-tech/autodev/internal/contracts"
)

const (
	RuntimeImagePrefix   = "autodev-stage"
	RuntimeBaseImageName = "autodev-stage-base"
)

type RuntimeImage struct {
	Stage  string
	Ref    string
	Digest string
}

func StageImageRef(stageName string) string {
	return fmt.Sprintf("%s-%s:current", RuntimeImagePrefix, stageName)
}

func BaseImageRef() string {
	return fmt.Sprintf("%s-base:current", RuntimeImagePrefix)
}

func ResolveRuntimeImage(stageName string) (RuntimeImage, error) {
	ref := StageImageRef(stageName)
	digest, err := inspectImageDigest(ref)
	if err != nil {
		return RuntimeImage{}, err
	}
	return RuntimeImage{
		Stage:  stageName,
		Ref:    ref,
		Digest: digest,
	}, nil
}

func ResolveRuntimeImages(stageNames []string) (map[string]RuntimeImage, error) {
	images := make(map[string]RuntimeImage, len(stageNames))
	for _, stage := range stageNames {
		image, err := ResolveRuntimeImage(stage)
		if err != nil {
			return nil, err
		}
		images[stage] = image
	}
	return images, nil
}

func inspectImageDigest(ref string) (string, error) {
	output, err := exec.Command("docker", "image", "inspect", ref, "--format", "{{json .RepoDigests}}").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect runtime image %s: %w\n%s", ref, err, strings.TrimSpace(string(output)))
	}
	digestsOutput := strings.TrimSpace(string(output))
	if digestsOutput != "" && digestsOutput != "null" && digestsOutput != "[]" {
		var digests []string
		if err := contracts.Unmarshal([]byte(digestsOutput), "", "docker inspect repo digests", &digests); err == nil && len(digests) > 0 {
			if parts := strings.SplitN(digests[0], "@", 2); len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	idOutput, err := exec.Command("docker", "image", "inspect", ref, "--format", "{{.Id}}").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect runtime image id %s: %w\n%s", ref, err, strings.TrimSpace(string(idOutput)))
	}
	digest := strings.TrimSpace(string(idOutput))
	if digest == "" {
		return "", fmt.Errorf("runtime image %s returned empty image id", ref)
	}
	return digest, nil
}
