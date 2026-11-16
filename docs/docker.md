# Docker builds in Docker places

Set `dockerInDocker` on a Docker place to let its jobs use `docker build`,
`docker buildx`, `docker login`, and other Docker commands. For example:

```json
{"id": "build-1", "provider": "docker", "pools": ["linux-build"],
 "cores": 4, "memoryMB": 8192, "dockerInDocker": true}
```

If the host resolves through a loopback stub such as systemd-resolved, set
`dockerDNS` to the host's reachable DNS server addresses (for example
`["192.0.2.53"]`). Each address is given to both the sidecar and runner.

The default runner image includes the Docker CLI and buildx. runner-up starts
`docker:28-dind` as a privileged sidecar on a new network for each job, waits
for its daemon, and gives the runner `DOCKER_HOST=tcp://docker:2375`. No host
Docker socket is mounted into the runner. Cleanup removes the runner, sidecar,
and network. The runner gets one core and 1024 MB; the daemon and its build
containers get the rest of the place's reservation. Set at least two cores and
2048 MB.

The sidecar is privileged and shares the host kernel. A container escape could
reach the machine, so use this only for jobs you trust and keep the fork guard
enabled. The daemon's unencrypted TCP port is on the job's private network,
but job code can control that daemon and every container it creates.
