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
	"strings"
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

func isLinuxAMD64(path string) bool {
	f, err := elf.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	return f.Machine == elf.EM_X86_64
}

func docker(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	hideConsole(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func dockerOK(args ...string) bool {
	cmd := exec.Command("docker", args...)
	hideConsole(cmd)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	return cmd.Run() == nil
}

// DockerCommandContext exposes a docker invocation for callers that need to
// stream stdio themselves (the agent driver). It keeps "how we reach the
// environment" in one package.
func DockerCommandContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "docker", args...)
	hideConsole(cmd)
	return cmd
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

func Up() error {
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
	if _, err := docker("volume", "create", Volume); err != nil {
		return err
	}
	if dockerOK("container", "inspect", Container) {
		if _, err := docker("start", Container); err != nil {
			return err
		}
	} else if _, err := docker("run", "-d", "--name", Container, "--restart", "unless-stopped",
		"-v", Volume+":"+Home, Image, "sleep", "infinity"); err != nil {
		return err
	}
	if err := seed(); err != nil {
		return err
	}
	fmt.Println("zeroclaw environment is up")
	return nil
}

// seed populates the agent home on first run and is a no-op afterwards.
func seed() error {
	script := strings.Join([]string{
		"set -e",
		"mkdir -p ~/.config/zero ~/memory ~/workspace ~/incoming ~/outgoing",
		"[ -e ~/.config/zero/ZERO.md ] || cp /opt/zeroclaw/bootstrap/ZEROCLAW.md ~/.config/zero/ZERO.md",
		"[ -e ~/MEMORY.md ] || cp /opt/zeroclaw/bootstrap/MEMORY.md ~/MEMORY.md",
		"[ -e ~/HEARTBEAT.md ] || cp /opt/zeroclaw/bootstrap/HEARTBEAT.md ~/HEARTBEAT.md",
	}, " && ")
	if _, err := docker("exec", Container, "sh", "-c", script); err != nil {
		return err
	}
	if err := adoptZeroAuth(); err != nil {
		return err
	}
	return allowSandboxNetwork()
}

// allowSandboxNetwork opens zero's inner network sandbox inside the container.
// The host config adopted by adoptZeroAuth carries the host's default (deny),
// but in here the container is the isolation boundary, so denying egress only
// strands the agent (it cannot reach GitHub while gh, git, and curl sit
// installed for exactly that). Only a missing setting is filled in: an
// operator who deliberately set "deny" in the agent's config keeps it.
func allowSandboxNetwork() error {
	script := `f=~/.config/zero/config.json
[ -e "$f" ] || exit 0
jq '.sandbox.network //= "allow"' "$f" > "$f.tmp" && mv "$f.tmp" "$f"`
	_, err := docker("exec", Container, "sh", "-c", script)
	return err
}

// adoptZeroAuth copies the host zero provider config and encrypted credential
// store into the agent's volume once, so the agent can talk to the same
// provider as the host zero install. Files are never overwritten on later ups.
func adoptZeroAuth() error {
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
		if dockerOK("exec", Container, "test", "-e", Home+"/.config/zero/"+f) {
			continue
		}
		if _, err := docker("cp", p, Container+":"+Home+"/.config/zero/"+f); err != nil {
			return err
		}
		copied = true
		fmt.Println("adopted host zero", f)
	}
	if !copied {
		return nil
	}
	// docker cp writes as root; hand the files to the agent user.
	_, err = docker("exec", "-u", "root", Container, "chown", "-R", "zeroclaw:zeroclaw", Home+"/.config/zero")
	return err
}

// Give copies a host file into the agent's ~/incoming. This and Take are the
// only sanctioned host-to-agent file paths; there are no bind mounts.
func Give(hostPath string) error {
	abs, err := filepath.Abs(hostPath)
	if err != nil {
		return err
	}
	if !fileExists(abs) {
		return fmt.Errorf("no such file: %s", abs)
	}
	dest := Home + "/incoming/" + filepath.Base(abs)
	if _, err := docker("cp", abs, Container+":"+dest); err != nil {
		return err
	}
	if _, err := docker("exec", "-u", "root", Container, "chown", "-R", "zeroclaw:zeroclaw", Home+"/incoming"); err != nil {
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

// takeScript resolves $1 to its real, symlink-free path inside the
// container, rejects it unless it is $2 (Home) or under it, then streams a
// tar of either the resolved path's contents ($3 == "1", docker cp's "copy
// directory contents" form) or the resolved path itself. Resolving and
// reading happen in this one process so a symlink cannot be swapped in
// between a separate check and a separate copy.
const takeScript = `p=$(readlink -f "$1") || exit 1
case "$p" in
  "$2"|"$2"/*) ;;
  *) exit 2 ;;
esac
if [ "$3" = "1" ]; then
  exec tar -cf - -C "$p" .
else
  exec tar -cf - -C "$(dirname "$p")" "$(basename "$p")"
fi`

// Take copies a file or directory out of the agent's home to a host path.
// Relative container paths are resolved against the agent home.
func Take(containerPath, hostDest string) error {
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

	cmd := DockerCommandContext(context.Background(), "exec", Container, "sh", "-c", takeScript, "sh", src, Home, contentsOnly)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	extractErr := extractTar(stdout, hostDest)
	// tar.Reader stops at the archive's end-of-archive marker, short of the
	// writer's block padding. Drain the rest so docker exec's write doesn't
	// block on a full pipe and leave cmd.Wait hanging forever.
	_, _ = io.Copy(io.Discard, stdout)
	if waitErr := cmd.Wait(); waitErr != nil {
		if cmd.ProcessState.ExitCode() == 2 {
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

// extractTar writes a tar stream into destDir, creating it if necessary.
func extractTar(r io.Reader, destDir string) error {
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
		target := filepath.Join(destDir, filepath.FromSlash(hdr.Name))
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		default:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, copyErr := io.Copy(f, tr); copyErr != nil {
				f.Close()
				return copyErr
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
}

func Down() error {
	_, err := docker("stop", Container)
	return err
}

// Reset removes the container and the volume: the agent's entire world.
// The CLI requires --force before calling this.
func Reset() error {
	if dockerOK("container", "inspect", Container) {
		if _, err := docker("rm", "-f", Container); err != nil {
			return err
		}
	}
	if dockerOK("volume", "inspect", Volume) {
		if _, err := docker("volume", "rm", Volume); err != nil {
			return err
		}
	}
	fmt.Println("zeroclaw environment removed")
	return nil
}

func Status(w io.Writer) error {
	if err := EngineReady(); err != nil {
		return err
	}
	state := "absent"
	if dockerOK("container", "inspect", Container) {
		out, err := docker("inspect", "-f", "{{.State.Status}}", Container)
		if err != nil {
			return err
		}
		state = out
	}
	vol := dockerOK("volume", "inspect", Volume)
	fmt.Fprintf(w, "container: %s\nvolume:    %v\n", state, vol)
	return nil
}

func Doctor(w io.Writer) error {
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
	running := dockerOK("exec", Container, "true")
	check("container "+Container+" running", running, "zeroclaw up")
	if running {
		if backendDoctor != nil {
			backendDoctor(w, Container)
		}
		check("zero credentials adopted", dockerOK("exec", Container, "test", "-e", Home+"/.config/zero/credentials.enc"), "zeroclaw up copies them from the host zero config")
	}
	dir, err := envDir()
	check("env build context", err == nil, "run from the zeroclaw repo")
	if err == nil {
		zeroBin := filepath.Join(dir, "bin", "zero")
		check("env/bin/zero (linux build)", isLinuxAMD64(zeroBin), "cross-compile zero for linux/amd64")
		cairnBin := filepath.Join(dir, "bin", "cairn-code")
		if fileExists(cairnBin) {
			if isLinuxAMD64(cairnBin) {
				check("env/bin/cairn-code (linux build)", true, "")
			} else {
				check("env/bin/cairn-code (artifact present, not linux/amd64)", false, "cross-compile cairn-code for linux/amd64")
			}
		}
	}
	fmt.Fprintln(w, "note: running without hard isolation is not supported yet; docker is required (tier 3 fallback is an M4 item)")
	return nil
}
