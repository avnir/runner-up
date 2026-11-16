package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultDockerImage is GitHub's own runner image, pinned to the runner
// version every place uses.
const DefaultDockerImage = "ghcr.io/actions/actions-runner:" + runnerVersion

// dockerProvider runs each job in a new container, removed after the job. The
// container is the place: one runner, one job, nothing kept.
type dockerProvider struct{}

func containerName(place string) string { return "runner-up-" + place }
func daemonName(place string) string    { return containerName(place) + "-docker" }

// dockerEntry starts the runner inside the container: it reads the
// registration from standard input and gives it only to the runner, in the
// one variable the runner reads it from. The runner removes it from its
// environment before any job starts.
const dockerEntry = `read -r ` + registrationVar + ` && export ` + registrationVar + ` && exec "$@"`

func (p dockerProvider) run(ctx context.Context, a *AgentRun, jit string, out io.Writer) error {
	l := p.launch(a)
	hook := filepath.Join(a.Dir(), "fork-guard.sh")
	if l.Hook != "" {
		if err := os.WriteFile(hook, []byte(l.Guard), 0o755); err != nil {
			return err
		}
	}
	if a.DockerInDocker {
		if output, err := exec.CommandContext(ctx, "docker", "network", "create", containerName(a.Place)).CombinedOutput(); err != nil {
			return fmt.Errorf("docker network create: %s", FirstLine(string(output), err))
		}
		args := []string{"run", "-d", "--rm", "--privileged", "--name", daemonName(a.Place), "--network", containerName(a.Place),
			"--network-alias", "docker", "--cpus", strconv.Itoa(a.Cores - 1), "--memory", strconv.Itoa(a.MemoryMB-1024) + "m",
			"-e", "DOCKER_TLS_CERTDIR=", "docker:28-dind"}
		if output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("docker daemon start: %s", FirstLine(string(output), err))
		}
		ready, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		for {
			if err := exec.CommandContext(ready, "docker", "exec", daemonName(a.Place), "docker", "info").Run(); err == nil {
				break
			}
			if err := ready.Err(); err != nil {
				return fmt.Errorf("docker daemon did not become ready: %w", err)
			}
			time.Sleep(time.Second)
		}
	}
	cmd := exec.Command("docker", dockerArgs(a, l, hook)...)
	cmd.Stdin = strings.NewReader(jit + "\n")
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("docker: %w", err)
	}
	stop := func() {
		c, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		exec.CommandContext(c, "docker", "rm", "-f", containerName(a.Place)).Run()
	}
	err := waitOrStop(ctx, cmd, stop, time.Minute)
	if ctx.Err() != nil {
		return nil
	}
	if code := ExitCode(err); code == 125 {
		// docker's own failure, such as a missing image: no runner ran.
		return fmt.Errorf("docker could not start the container")
	}
	return nil
}

// dockerArgs run the place's container, with the fork guard's hook file,
// written on the machine at hook, mounted where l names it. -i keeps the
// container's standard input open: the registration arrives there, never in
// docker's arguments or environment, so the container's settings (docker
// inspect) never hold it.
func dockerArgs(a *AgentRun, l runnerLaunch, hook string) []string {
	image := a.Image
	if image == "" {
		image = DefaultDockerImage
	}
	cores, memory := a.Cores, a.MemoryMB
	if a.DockerInDocker {
		cores, memory = 1, 1024
	}
	args := []string{"run", "--rm", "-i", "--name", containerName(a.Place), "--label", "runner-up.place=" + a.Place,
		"--cpus", strconv.Itoa(cores), "--memory", strconv.Itoa(memory) + "m"}
	if a.DockerInDocker {
		args = append(args, "--network", containerName(a.Place), "-e", "DOCKER_HOST=tcp://docker:2375")
	}
	if l.Hook != "" {
		args = append(args, "-v", a.Self+":"+dockerGuest+"/runner-up:ro", "-v", hook+":"+l.Hook+":ro")
	}
	for _, env := range l.Env {
		args = append(args, "-e", env)
	}
	return append(args, "--entrypoint", "/bin/sh", image, "-c", dockerEntry, "runner-up", "/home/runner/run.sh")
}

func (dockerProvider) clean(ctx context.Context, a *AgentRun) error {
	var problems []error
	for _, name := range []string{containerName(a.Place), daemonName(a.Place)} {
		out, err := exec.CommandContext(ctx, "docker", "rm", "-f", name).CombinedOutput()
		if err != nil && !strings.Contains(string(out), "No such container") {
			problems = append(problems, fmt.Errorf("docker rm: %s", FirstLine(string(out), err)))
		}
	}
	out, err := exec.CommandContext(ctx, "docker", "network", "rm", containerName(a.Place)).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "not found") {
		problems = append(problems, fmt.Errorf("docker network rm: %s", FirstLine(string(out), err)))
	}
	return errors.Join(problems...)
}
