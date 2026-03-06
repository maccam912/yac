// Package docker implements yac.Sandbox by running Python code in
// ephemeral Docker containers.
package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/maccam912/yac"
)

// DefaultImage is the Python Docker image used if none is specified.
const DefaultImage = "python:3.12-slim"

// Sandbox runs Python code in Docker containers.
type Sandbox struct {
	Image string // Docker image; defaults to DefaultImage
}

func (s *Sandbox) image() string {
	if s.Image != "" {
		return s.Image
	}
	return DefaultImage
}

// Exec runs the given Python code in a fresh container and returns its
// stdout/stderr output.
func (s *Sandbox) Exec(ctx context.Context, code string) (*yac.ExecResult, error) {
	cmd := exec.CommandContext(ctx, "docker", "run",
		"--rm",
		"-i",
		"--network", "none",
		"--memory", "256m",
		"--cpus", "1",
		"--pids-limit", "64",
		"--read-only",
		s.image(),
		"python", "-c", code,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := &yac.ExecResult{
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
	}

	if err == nil {
		return result, nil
	}

	result.ExitCode = 1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}

	return result, fmt.Errorf("docker exec: %w", err)
}

// Close is a no-op for the basic Docker sandbox since each Exec call
// uses a fresh container. It exists to satisfy the yac.Sandbox interface.
func (s *Sandbox) Close() error {
	return nil
}
