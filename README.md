# kured-toujours

Request a periodic reboot by creating a sentinel file once a Linux node reaches a
configured uptime. [kured](https://github.com/kubereboot/kured) watches that file,
coordinates draining, and performs the reboot. `kured-toujours` only creates the
marker; it does not drain nodes or reboot them itself.

## Build and try safely

Use Linux and Go 1.27.1 (the version declared in `go.mod`):

```sh
git clone https://github.com/barpilot/kured-toujours.git
cd kured-toujours
go mod download
go mod verify
go test -race ./...
go build -mod=readonly -trimpath -o kured-toujours .

# Create a marker in a disposable directory, without requesting a host reboot.
sentinel_dir=$(mktemp -d)
./kured-toujours --period 1s --reboot-sentinel "$sentinel_dir/reboot-required"
```

After the log reports `Reboot sentinel present`, press Ctrl+C. Inspect and remove
the disposable directory from the same shell:

```sh
ls -l "$sentinel_dir/reboot-required"
rm -rf "$sentinel_dir"
```

## Timing and configuration

- `--period` is the minimum node uptime before requesting a reboot (default `1h`;
  use `168h` for one week). It must be positive.
- `--reboot-sentinel` is an absolute path in the daemon's filesystem (default
  `/var/run/reboot-required`). Its parent directory must already exist and be
  writable. Use a directory controlled by the administrator; parent directories
  must be trusted. A final symlink or nonregular file is rejected.

The daemon waits for `period - uptime`, or creates the marker immediately if the
node is already due. It ensures the marker exists again every `period` while
waiting for the reboot controller. Existing file contents are preserved. A marker
write failure exits with an error; SIGINT and SIGTERM stop the daemon cleanly.
Node uptime comes from Linux `sysinfo(2)`, not the container start time; this
assumes the container uses the host's normal time namespace.

Configuration is through these flags. The former `--config` and `--toggle`
scaffolding did not configure the timer or marker and has been removed. The former
random initial delay could request a reboot before the uptime threshold; requests
now begin at the threshold. Overdue nodes are marked immediately rather than
waiting another full period.

## Container

```sh
docker build -t kured-toujours:local .
sentinel_dir=$(mktemp -d)
docker run --rm --user "$(id -u):$(id -g)" --read-only --cap-drop=ALL \
  --security-opt=no-new-privileges \
  -v "$sentinel_dir:/sentinel" \
  kured-toujours:local --period 1s --reboot-sentinel /sentinel/reboot-required
```

Press Ctrl+C, then inspect and remove `$sentinel_dir` as above. The image contains
only the static binary and requires no `nsenter`, `touch`, shell, or privileged
container. It runs as root so it can create the marker in a root-owned host
directory when deployed; all Linux capabilities can be dropped.

## Kubernetes deployment

Install and configure [kured](https://kured.dev/) first. Its reboot sentinel must
match the host file used by this daemon. Build/load `kured-toujours:local` into your
local cluster, or change the image in `daemonset.yaml` to a published tag or digest
before applying it:

```sh
kubectl apply -f daemonset.yaml
```

The example requests a reboot after `168h`. It mounts the existing host `/var/run`
directory at `/host/run` and writes only `/host/run/reboot-required`. There is no
host PID namespace, privileged mode, or Kubernetes API credential. The writable
host directory mount grants root read/write access to all of host `/var/run`,
including any runtime sockets it contains. Deploy only trusted images with this
access. To narrow the mount, create a dedicated directory under the host's
temporary `/run` filesystem and configure both daemons to use the same sentinel.
The directory must be recreated after boot, and its marker must disappear on
reboot to avoid repeated reboot requests.

Mount the **directory**, not a `FileOrCreate` sentinel: precreating the marker
would request a reboot at deployment time. The local quick start and tests use
disposable directories; applying the DaemonSet intentionally requests real node
reboots once the threshold is reached.

## Validation and releases

```sh
go mod verify
go mod tidy -diff
go vet ./...
go test -race -count=1 ./...
CGO_ENABLED=0 go build -mod=readonly -trimpath .
```

Tests cover the timer, cancellation, validation, and sentinel boundaries using
temporary directories and the Go standard library's virtual clock. CI also builds
the image, creates a marker in a temporary bind mount, and checks graceful shutdown.

Tag releases (`v*`) run the same CI checks before GoReleaser 2.18.2 publishes Linux
archives and checksums for 386, amd64, ARMv6, ARMv7, and arm64. The workflow then
publishes matching container platforms to
`ghcr.io/barpilot/kured-toujours:<tag>` using the repository's GitHub token; GitHub
Packages visibility must permit your nodes to pull the image. It does not publish
an unversioned `latest` image. Builds use pinned modules, a pinned Go container
digest, and immutable action references. Release tooling can be checked locally
without publishing:

```sh
goreleaser check
goreleaser release --snapshot --clean --skip=publish
```

The legacy Travis release pipeline has been replaced by GitHub Actions. GitHub
release and container publication use separate jobs. If the container upload
fails after archives were published, rerun only the failed container job.
