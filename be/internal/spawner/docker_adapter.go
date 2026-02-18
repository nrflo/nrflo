package spawner

import (
	"fmt"
	"os/exec"
)

// DockerConfig holds configuration for Docker-based agent isolation.
type DockerConfig struct {
	ProjectRoot  string // Original project root (mount target inside container)
	WorktreePath string // Worktree path on host (empty if no worktree)
	HomeDir      string // Host home directory
	UID          int    // Host user ID (passed as HOST_UID)
	GID          int    // Host group ID (passed as HOST_GID)
}

// DockerCLIAdapter wraps a CLIAdapter and transforms its commands into docker run invocations.
type DockerCLIAdapter struct {
	inner  CLIAdapter
	config DockerConfig
}

// NewDockerCLIAdapter creates a new DockerCLIAdapter wrapping the given inner adapter.
func NewDockerCLIAdapter(inner CLIAdapter, config DockerConfig) *DockerCLIAdapter {
	return &DockerCLIAdapter{inner: inner, config: config}
}

func (a *DockerCLIAdapter) Name() string                    { return a.inner.Name() }
func (a *DockerCLIAdapter) MapModel(model string) string    { return a.inner.MapModel(model) }
func (a *DockerCLIAdapter) SupportsSessionID() bool         { return a.inner.SupportsSessionID() }
func (a *DockerCLIAdapter) SupportsSystemPromptFile() bool  { return a.inner.SupportsSystemPromptFile() }
func (a *DockerCLIAdapter) SupportsResume() bool            { return a.inner.SupportsResume() }
func (a *DockerCLIAdapter) UsesStdinPrompt() bool           { return a.inner.UsesStdinPrompt() }

func (a *DockerCLIAdapter) BuildCommand(opts SpawnOptions) *exec.Cmd {
	innerCmd := a.inner.BuildCommand(opts)
	args := a.buildDockerArgs(opts.SessionID, opts.Env, false)
	// Append the inner command (Path + remaining args after the binary name)
	args = append(args, innerCmd.Path)
	args = append(args, innerCmd.Args[1:]...)

	cmd := exec.Command("docker", args...)
	cmd.Dir = a.hostDir()
	cmd.Env = innerCmd.Env
	return cmd
}

func (a *DockerCLIAdapter) BuildResumeCommand(opts ResumeOptions) *exec.Cmd {
	innerCmd := a.inner.BuildResumeCommand(opts)
	if innerCmd == nil {
		return nil
	}
	args := a.buildDockerArgs(opts.SessionID, opts.Env, true)
	args = append(args, innerCmd.Path)
	args = append(args, innerCmd.Args[1:]...)

	cmd := exec.Command("docker", args...)
	cmd.Dir = a.hostDir()
	cmd.Env = innerCmd.Env
	return cmd
}

// buildDockerArgs constructs the docker run arguments (everything before the inner command).
func (a *DockerCLIAdapter) buildDockerArgs(sessionID string, env []string, isResume bool) []string {
	containerName := a.containerName(sessionID, isResume)

	args := []string{
		"run", "--rm",
		"--platform", "linux/arm64",
		"--name", containerName,
	}

	// Environment variables
	args = append(args, "-e", fmt.Sprintf("HOST_UID=%d", a.config.UID))
	args = append(args, "-e", fmt.Sprintf("HOST_GID=%d", a.config.GID))
	args = append(args, "-e", "TMPDIR=/tmp")
	for _, e := range env {
		args = append(args, "-e", e)
	}

	// Volume mounts
	for _, m := range a.volumeMounts() {
		args = append(args, "-v", m)
	}

	// Working directory inside container
	args = append(args, "-w", a.config.ProjectRoot)

	// Image
	args = append(args, "nrworkflow-agent")

	return args
}

// volumeMounts returns the list of -v mount specifications.
func (a *DockerCLIAdapter) volumeMounts() []string {
	home := a.config.HomeDir
	mounts := []string{
		fmt.Sprintf("%s/.claude:%s/.claude", home, home),
		fmt.Sprintf("%s/.config/opencode:%s/.config/opencode", home, home),
		fmt.Sprintf("%s/.local/share/opencode:%s/.local/share/opencode", home, home),
		"/tmp/nrworkflow:/tmp/nrworkflow",
		fmt.Sprintf("%s/.ai_common/safety.json:%s/.ai_common/safety.json:ro", home, home),
	}

	// Project directory mount (worktree-aware)
	if a.config.WorktreePath != "" {
		// Worktree: mount host worktree at the original project root path inside container
		mounts = append(mounts, fmt.Sprintf("%s:%s", a.config.WorktreePath, a.config.ProjectRoot))
	} else {
		mounts = append(mounts, fmt.Sprintf("%s:%s", a.config.ProjectRoot, a.config.ProjectRoot))
	}

	return mounts
}

// containerName returns the Docker container name for this session.
func (a *DockerCLIAdapter) containerName(sessionID string, isResume bool) string {
	id := sessionID
	if len(id) > 12 {
		id = id[:12]
	}
	if isResume {
		return "nrwf-" + id + "-rsm"
	}
	return "nrwf-" + id
}

// hostDir returns the host-side working directory.
func (a *DockerCLIAdapter) hostDir() string {
	if a.config.WorktreePath != "" {
		return a.config.WorktreePath
	}
	return a.config.ProjectRoot
}

// ContainerName returns the Docker container name for a given session ID.
func (a *DockerCLIAdapter) ContainerName(sessionID string) string {
	return a.containerName(sessionID, false)
}

// StopContainer force-removes a running Docker container by name.
func StopContainer(name string) {
	if name == "" {
		return
	}
	exec.Command("docker", "rm", "-f", name).Run()
}

// CleanupStaleContainers removes any leftover containers with the nrwf- name prefix.
func CleanupStaleContainers() {
	exec.Command("sh", "-c", "docker ps -a --filter name=nrwf- -q | xargs -r docker rm -f").Run()
}
