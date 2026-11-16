package coordinator

import (
	"slices"
	"testing"

	"github.com/kunchenguid/runner-up/internal/config"
)

// Intent (docs/docker.md): the setup opt-in reaches the agent for a job of
// that place, while existing Docker places keep their original arguments.
func TestDockerInDockerRunArgs(t *testing.T) {
	p := config.Place{ID: "build-1", Provider: config.ProviderDocker, Cores: 2, MemoryMB: 2048}
	cfg := &config.Config{}
	if slices.Contains(RunArgs(cfg, p, "linux-build"), "--docker-in-docker") {
		t.Fatal("the default Docker place enabled Docker in Docker")
	}
	p.DockerInDocker = true
	if !slices.Contains(RunArgs(cfg, p, "linux-build"), "--docker-in-docker") {
		t.Fatal("the opted-in Docker place did not enable Docker in Docker")
	}
	p.DockerDNS = []string{"192.0.2.53"}
	args := RunArgs(cfg, p, "linux-build")
	if !slices.Contains(args, "--docker-dns") || !slices.Contains(args, "192.0.2.53") {
		t.Fatal("the configured Docker DNS did not reach the agent")
	}
}
