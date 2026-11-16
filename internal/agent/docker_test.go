package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Intent (README, Docker places): an opted-in job gets a private daemon and
// network, its runner reaches that daemon without the host socket, and cleanup
// removes both containers and the network before the place is reused.
func TestDockerInDockerPlace(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "commands")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	a := &AgentRun{Place: "slot-1", Cores: 3, MemoryMB: 4096, DockerInDocker: true,
		AllowForks: true, StateDir: dir}
	p := dockerProvider{}
	if err := p.run(context.Background(), a, "registration", os.Stdout); err != nil {
		t.Fatal(err)
	}
	if err := p.clean(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	want := []string{
		"network create runner-up-slot-1",
		"run -d --rm --privileged --name runner-up-slot-1-docker --network runner-up-slot-1 --network-alias docker --cpus 2 --memory 3072m -e DOCKER_TLS_CERTDIR= docker:28-dind",
		"exec runner-up-slot-1-docker docker info",
		"run --rm -i --name runner-up-slot-1 --label runner-up.place=slot-1 --cpus 1 --memory 1024m --network runner-up-slot-1 -e DOCKER_HOST=tcp://docker:2375 -e RUNNER_UP_CORES=3 -e RUNNER_UP_MEMORY_MB=4096 --entrypoint /bin/sh " + DefaultDockerImage + ` -c read -r ACTIONS_RUNNER_INPUT_JITCONFIG && export ACTIONS_RUNNER_INPUT_JITCONFIG && exec "$@" runner-up /home/runner/run.sh`,
		"rm -f runner-up-slot-1", "rm -f runner-up-slot-1-docker", "network rm runner-up-slot-1",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("docker commands:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if strings.Contains(string(b), "/var/run/docker.sock") || strings.Contains(string(b), "registration") {
		t.Errorf("Docker command exposed the host socket or registration: %s", b)
	}
}

func TestDockerInDockerDNS(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "commands")
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	a := &AgentRun{Place: "slot-1", Cores: 3, MemoryMB: 4096, DockerInDocker: true, DockerDNS: []string{"192.0.2.53", "2001:db8::53"}, AllowForks: true, StateDir: dir}
	if err := (dockerProvider{}).run(context.Background(), a, "registration", os.Stdout); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if !strings.Contains(lines[1], "--dns 192.0.2.53 --dns 2001:db8::53 docker:28-dind") || !strings.Contains(lines[3], "--dns 192.0.2.53 --dns 2001:db8::53") {
		t.Fatalf("docker DNS arguments:\n%s", string(b))
	}
}
