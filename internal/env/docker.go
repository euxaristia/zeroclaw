// Package env owns the zeroclaw environment lifecycle: the container that is
// the agent's isolation boundary and the named volume that is its entire
// persistent world. Isolation rule: never bind-mount host paths; file exchange
// happens only through explicit docker cp (give/take, and the one-time
// adoption of host zero credentials during up).
package env

import (
	"archive/tar"
	"bytes"
	"context"
	"debug/elf"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"zeroclaw/internal/config"
)

const (
	Image     = "zeroclaw-env"
	Container = "zeroclaw"
	Volume    = "zeroclaw-home"
	Home      = "/home/zeroclaw"
)

var backendDoctor func(w io.Writer, container string)

// RegisterBackendDoctor registers a callback for backend-specific health checks inside the container.
func RegisterBackendDoctor(fn func(w io.Writer, container string)) {
	backendDoctor = fn
}

// ContainerName returns the Docker container name for the given agent profile.
func ContainerName(agent ...string) string {
	name := "default"
	if len(agent) > 0 && agent[0] != "" {
		name = agent[0]
	}
	if name == "default" {
		return Container
	}
	return "zeroclaw-" + name
}

// VolumeName returns the Docker named volume for the given agent profile.
func VolumeName(agent ...string) string {
	name := "default"
	if len(agent) > 0 && agent[0] != "" {
		name = agent[0]
	}
	if name == "default" {
		return Volume
	}
	return "zeroclaw-" + name + "-home"
}

func isLinuxAMD64(path string) bool {
	f, err := elf.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return f.Machine == elf.EM_X86_64
}

func dockerCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "docker", args...)
	hideConsole(cmd)
	cmd.Env = append(os.Environ(), "DOCKER_CLI_HINTS=false")
	return cmd
}

func docker(args ...string) (string, error) {
	cmd := dockerCmd(context.Background(), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func dockerOK(args ...string) bool {
	cmd := dockerCmd(context.Background(), args...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	return cmd.Run() == nil
}

// DockerCommandContext exposes a docker invocation for callers that need to
// stream stdio themselves (the agent driver). It keeps "how we reach the
// environment" in one package.
func DockerCommandContext(ctx context.Context, args ...string) *exec.Cmd {
	return dockerCmd(ctx, args...)
}

func EngineReady() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker CLI not found on PATH")
	}
	if !dockerOK("info") {
		return fmt.Errorf("docker engine is not responding; start Docker Desktop and retry")
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// envDir locates the docker build context (env/Dockerfile plus bootstrap and
// the cross-compiled zero binary), next to the executable or under the cwd.
func envDir() (string, error) {
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "env")
		if fileExists(filepath.Join(dir, "Dockerfile")) {
			return dir, nil
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(wd, "env")
	if fileExists(filepath.Join(dir, "Dockerfile")) {
		return dir, nil
	}
	return "", fmt.Errorf("cannot find env/Dockerfile next to the zeroclaw executable or in the current directory")
}

func Up(agent ...string) error {
	if err := EngineReady(); err != nil {
		return err
	}
	dir, err := envDir()
	if err != nil {
		return err
	}
	zeroBin := filepath.Join(dir, "bin", "zero")
	if !fileExists(zeroBin) {
		return fmt.Errorf("missing %s; cross-compile zero for linux/amd64 into env/bin first", zeroBin)
	}
	if !dockerOK("image", "inspect", Image) {
		fmt.Println("building image", Image)
		build := exec.Command("docker", "build", "-t", Image, dir)
		hideConsole(build)
		build.Stdout, build.Stderr = os.Stdout, os.Stderr
		if err := build.Run(); err != nil {
			return fmt.Errorf("image build failed: %w", err)
		}
	}

	container := ContainerName(agent...)
	volume := VolumeName(agent...)

	if _, err := docker("volume", "create", volume); err != nil {
		return err
	}
	if dockerOK("container", "inspect", container) {
		if _, err := docker("start", container); err != nil {
			return err
		}
	} else if _, err := docker("run", "-d", "--name", container, "--restart", "unless-stopped",
		"-v", volume+":"+Home, Image, "sleep", "infinity"); err != nil {
		return err
	}
	if err := seed(container); err != nil {
		return err
	}
	fmt.Printf("zeroclaw environment (%s) is up\n", container)
	return nil
}

// seed populates the agent home on first run and is a no-op afterwards.
func seed(container string) error {
	script := strings.Join([]string{
		"set -e",
		"mkdir -p ~/.config/zero ~/memory ~/workspace ~/incoming ~/outgoing",
		"[ -e ~/.config/zero/ZERO.md ] || cp /opt/zeroclaw/bootstrap/ZEROCLAW.md ~/.config/zero/ZERO.md",
		"[ -e ~/MEMORY.md ] || cp /opt/zeroclaw/bootstrap/MEMORY.md ~/MEMORY.md",
		"[ -e ~/HEARTBEAT.md ] || cp /opt/zeroclaw/bootstrap/HEARTBEAT.md ~/HEARTBEAT.md",
	}, " && ")
	if _, err := docker("exec", container, "sh", "-c", script); err != nil {
		return err
	}
	if err := adoptZeroAuth(container); err != nil {
		return err
	}
	return allowSandboxNetwork(container)
}

// allowSandboxNetwork opens zero's inner network sandbox inside the container.
// The host config adopted by adoptZeroAuth carries the host's default (deny),
// but in here the container is the isolation boundary, so denying egress only
// strands the agent (it cannot reach GitHub while gh, git, and curl sit
// installed for exactly that). Only a missing setting is filled in: an
// operator who deliberately set "deny" in the agent's config keeps it.
func allowSandboxNetwork(container string) error {
	script := `f=~/.config/zero/config.json
[ -e "$f" ] || exit 0
jq '.sandbox.network //= "allow"' "$f" > "$f.tmp" && mv "$f.tmp" "$f"`
	_, err := docker("exec", container, "sh", "-c", script)
	return err
}

// adoptZeroAuth copies the host zero provider config and encrypted credential
// store into the agent's volume once, so the agent can talk to the same
// provider as the host zero install. Files are never overwritten on later ups.
func adoptZeroAuth(container string) error {
	hostCfg, err := os.UserConfigDir()
	if err != nil {
		return nil
	}
	src := filepath.Join(hostCfg, "zero")
	copied := false
	for _, f := range []string{"config.json", "credentials.enc", "credentials.enc.secret"} {
		p := filepath.Join(src, f)
		if !fileExists(p) {
			continue
		}
		if dockerOK("exec", container, "test", "-e", Home+"/.config/zero/"+f) {
			continue
		}
		if _, err := docker("cp", p, container+":"+Home+"/.config/zero/"+f); err != nil {
			return err
		}
		copied = true
		fmt.Println("adopted host zero", f)
	}
	if !copied {
		return nil
	}
	// docker cp writes as root; hand the files to the agent user.
	_, err = docker("exec", "-u", "root", container, "chown", "-R", "zeroclaw:zeroclaw", Home+"/.config/zero")
	return err
}

// SyncAuth explicitly copies the host zero provider config and encrypted credential
// store into the agent's volume, overwriting container zero auth files if present.
func SyncAuth(agent ...string) error {
	container := ContainerName(agent...)
	hostCfg, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("getting user config dir: %w", err)
	}
	src := filepath.Join(hostCfg, "zero")
	copied := 0
	for _, f := range []string{"config.json", "credentials.enc", "credentials.enc.secret"} {
		p := filepath.Join(src, f)
		if !fileExists(p) {
			continue
		}
		if _, err := docker("cp", p, container+":"+Home+"/.config/zero/"+f); err != nil {
			return fmt.Errorf("copying %s: %w", f, err)
		}
		copied++
		fmt.Println("synced host zero", f)
	}
	if copied == 0 {
		return fmt.Errorf("no host zero configuration found in %s", src)
	}
	if _, err := docker("exec", "-u", "root", container, "chown", "-R", "zeroclaw:zeroclaw", Home+"/.config/zero"); err != nil {
		return fmt.Errorf("chown zero config: %w", err)
	}
	return allowSandboxNetwork(container)
}

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// InteractiveAuth launches an interactive zero setup or zero auth session inside the container.
func InteractiveAuth(args []string, agent ...string) error {
	container := ContainerName(agent...)
	if len(args) > 0 && args[0] == "sync" {
		return SyncAuth(agent...)
	}
	execFlags := "-i"
	if isTerminal(os.Stdin) {
		execFlags = "-it"
	}
	var zeroCmd []string
	if len(args) == 0 {
		zeroCmd = []string{"exec", execFlags, container, "zero", "setup"}
	} else {
		zeroCmd = append([]string{"exec", execFlags, container, "zero", "auth"}, args...)
	}
	cmd := exec.Command("docker", zeroCmd...)
	cmd.Env = append(os.Environ(), "DOCKER_CLI_HINTS=false")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Give copies a host file into the agent's ~/incoming. This and Take are the
// only sanctioned host-to-agent file paths; there are no bind mounts.
func Give(hostPath string, agent ...string) error {
	container := ContainerName(agent...)
	abs, err := filepath.Abs(hostPath)
	if err != nil {
		return err
	}
	if !fileExists(abs) {
		return fmt.Errorf("no such file: %s", abs)
	}
	dest := Home + "/incoming/" + filepath.Base(abs)
	if _, err := docker("cp", abs, container+":"+dest); err != nil {
		return err
	}
	if _, err := docker("exec", "-u", "root", container, "chown", "-R", "zeroclaw:zeroclaw", Home+"/incoming"); err != nil {
		return err
	}
	fmt.Println("gave", filepath.Base(abs), "->", dest)
	return nil
}

// sanitizeContainerPath resolves a user-supplied container path against Home
// and rejects any path that would escape it. path.Clean strips a trailing
// "/.", which docker cp treats specially (copy the directory's contents
// rather than the directory itself), so that marker is preserved separately.
func sanitizeContainerPath(containerPath string) (string, error) {
	trailingDot := containerPath == "." || strings.HasSuffix(containerPath, "/.")
	clean := containerPath
	if !path.IsAbs(clean) {
		clean = path.Join(Home, clean)
	} else {
		clean = path.Clean(clean)
	}
	if clean != Home && !strings.HasPrefix(clean, Home+"/") {
		return "", fmt.Errorf("path traversal denied: %s is outside agent home", containerPath)
	}
	if trailingDot {
		clean += "/."
	}
	return clean, nil
}

const takeScript = `p=$(readlink -f "$1") || exit 1
case "$p" in
  "$2"|"$2"/*) ;;
  *) exit 91 ;;
esac
if [ "$3" = "1" ]; then
  exec tar -cf - -C "$p" .
else
  exec tar -cf - -C "$(dirname "$p")" "$(basename "$p")"
fi`

const exitCodeTraversalDenied = 91

// Take copies a file or directory out of the agent's home to a host path.
// Relative container paths are resolved against the agent home.
func Take(containerPath, hostDest string, agent ...string) error {
	container := ContainerName(agent...)
	clean, err := sanitizeContainerPath(containerPath)
	if err != nil {
		return err
	}
	if hostDest == "" {
		hostDest = "."
	}
	contentsOnly := "0"
	src := clean
	if strings.HasSuffix(clean, "/.") {
		contentsOnly = "1"
		src = strings.TrimSuffix(clean, "/.")
	}

	cmd := DockerCommandContext(context.Background(), "exec", container, "sh", "-c", takeScript, "sh", src, Home, contentsOnly)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return err
	}
	extractErr := extractTar(stdout, hostDest)
	_, _ = io.Copy(io.Discard, stdout)
	if waitErr := cmd.Wait(); waitErr != nil {
		if cmd.ProcessState.ExitCode() == exitCodeTraversalDenied {
			return fmt.Errorf("path traversal denied: %s resolves outside agent home", containerPath)
		}
		return fmt.Errorf("docker exec: %w\n%s", waitErr, strings.TrimSpace(stderr.String()))
	}
	if extractErr != nil {
		return fmt.Errorf("extracting %s: %w", containerPath, extractErr)
	}
	fmt.Println("took", clean, "->", hostDest)
	return nil
}

func extractTar(r io.Reader, destDir string) error {
	destDir, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			mode := os.FileMode(hdr.Mode) & os.ModePerm
			if err := os.MkdirAll(target, mode); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if _, err := safeJoin(filepath.Dir(target), hdr.Linkname); err != nil {
				return fmt.Errorf("tar entry %q: symlink target %q escapes destination: %w", hdr.Name, hdr.Linkname, err)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		case tar.TypeLink:
			return fmt.Errorf("tar entry %q: hard links are not supported", hdr.Name)
		default:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			mode := os.FileMode(hdr.Mode) & os.ModePerm
			f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, copyErr := io.Copy(f, tr); copyErr != nil {
				_ = f.Close()
				return copyErr
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
}

func safeJoin(destDir, name string) (string, error) {
	if path.IsAbs(filepath.ToSlash(name)) {
		return "", fmt.Errorf("%q is an absolute path", name)
	}
	joined := filepath.Join(destDir, filepath.FromSlash(name))
	rel, err := filepath.Rel(destDir, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q escapes destination directory", name)
	}
	return joined, nil
}

func Down(agent ...string) error {
	container := ContainerName(agent...)
	_, err := docker("stop", container)
	return err
}

// ResetContainer removes only the container, preserving the agent's volume and home directory.
func ResetContainer(agent ...string) error {
	container := ContainerName(agent...)
	if dockerOK("container", "inspect", container) {
		if _, err := docker("rm", "-f", container); err != nil {
			return err
		}
	}
	fmt.Printf("zeroclaw container %s removed (volume preserved)\n", container)
	return nil
}

// Reset removes the container and the volume: the agent's entire world.
// The CLI requires --force before calling this.
func Reset(agent ...string) error {
	container := ContainerName(agent...)
	volume := VolumeName(agent...)
	if dockerOK("container", "inspect", container) {
		if _, err := docker("rm", "-f", container); err != nil {
			return err
		}
	}
	if dockerOK("volume", "inspect", volume) {
		if _, err := docker("volume", "rm", volume); err != nil {
			return err
		}
	}
	fmt.Printf("zeroclaw environment (%s) removed\n", container)
	return nil
}

func Status(w io.Writer, agent ...string) error {
	if err := EngineReady(); err != nil {
		return err
	}
	container := ContainerName(agent...)
	volume := VolumeName(agent...)
	state := "absent"
	if dockerOK("container", "inspect", container) {
		out, err := docker("inspect", "-f", "{{.State.Status}}", container)
		if err != nil {
			return err
		}
		state = out
	}
	vol := dockerOK("volume", "inspect", volume)
	fmt.Fprintf(w, "container: %s (%s)\nvolume:    %v (%s)\n", state, container, vol, volume)
	return nil
}

func Doctor(w io.Writer, agent ...string) error {
	container := ContainerName(agent...)
	check := func(name string, ok bool, hint string) {
		mark := "ok  "
		if !ok {
			mark = "FAIL"
		}
		fmt.Fprintf(w, "%s %s", mark, name)
		if !ok && hint != "" {
			fmt.Fprintf(w, " (%s)", hint)
		}
		fmt.Fprintln(w)
	}
	_, lookErr := exec.LookPath("docker")
	check("docker CLI on PATH", lookErr == nil, "install Docker Desktop")
	engine := dockerOK("info")
	check("docker engine responding", engine, "start Docker Desktop")
	if !engine {
		return nil
	}
	check("image "+Image, dockerOK("image", "inspect", Image), "zeroclaw up builds it")
	running := dockerOK("exec", container, "true")
	check("container "+container+" running", running, "zeroclaw up")
	if running {
		if backendDoctor != nil {
			backendDoctor(w, container)
		}
		check("zero credentials adopted", dockerOK("exec", container, "test", "-e", Home+"/.config/zero/credentials.enc"), "zeroclaw up copies them from the host zero config")
	}
	dir, err := envDir()
	check("env build context", err == nil, "run from the zeroclaw repo")
	if err == nil {
		zeroBin := filepath.Join(dir, "bin", "zero")
		check("env/bin/zero (linux build)", isLinuxAMD64(zeroBin), "cross-compile zero for linux/amd64")
	}
	fmt.Fprintln(w, "note: running without hard isolation is not supported yet; docker is required (tier 3 fallback is an M4 item)")
	return nil
}

// AgentSummary represents status information for an agent instance.
type AgentSummary struct {
	Name            string
	Container       string
	Volume          string
	ContainerStatus string
	VolumePresent   bool
}

// DiscoverAgents finds all configured, containerized, or persisted agent instances.
func DiscoverAgents() ([]AgentSummary, error) {
	agentsMap := map[string]bool{"default": true}

	if cfgAgents, err := config.ConfiguredAgents(); err == nil {
		for _, a := range cfgAgents {
			if a != "" {
				agentsMap[a] = true
			}
		}
	}

	if EngineReady() == nil {
		// Inspect Docker containers
		if out, err := docker("ps", "-a", "--filter", "name=zeroclaw", "--format", "{{.Names}}"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line == "zeroclaw" {
					agentsMap["default"] = true
				} else if strings.HasPrefix(line, "zeroclaw-") {
					agentName := strings.TrimPrefix(line, "zeroclaw-")
					if agentName != "" {
						agentsMap[agentName] = true
					}
				}
			}
		}
		// Inspect Docker volumes
		if out, err := docker("volume", "ls", "--filter", "name=zeroclaw", "--format", "{{.Name}}"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				line = strings.TrimSpace(line)
				if line == "zeroclaw-home" {
					agentsMap["default"] = true
				} else if strings.HasPrefix(line, "zeroclaw-") && strings.HasSuffix(line, "-home") {
					agentName := strings.TrimSuffix(strings.TrimPrefix(line, "zeroclaw-"), "-home")
					if agentName != "" {
						agentsMap[agentName] = true
					}
				}
			}
		}
	}

	var names []string
	for a := range agentsMap {
		names = append(names, a)
	}
	sort.Strings(names)

	var summaries []AgentSummary
	for _, a := range names {
		cName := ContainerName(a)
		vName := VolumeName(a)
		cStatus := "absent"
		vPresent := false

		if dockerOK("container", "inspect", cName) {
			if out, err := docker("inspect", "-f", "{{.State.Status}}", cName); err == nil && out != "" {
				cStatus = out
			} else {
				cStatus = "present"
			}
		}
		if dockerOK("volume", "inspect", vName) {
			vPresent = true
		}

		summaries = append(summaries, AgentSummary{
			Name:            a,
			Container:       cName,
			Volume:          vName,
			ContainerStatus: cStatus,
			VolumePresent:   vPresent,
		})
	}
	return summaries, nil
}
