// Package docker implements yac.Sandbox by running Python code in
// ephemeral Docker containers.
package docker

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
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
// combined stdout+stderr output.
func (s *Sandbox) Exec(ctx context.Context, code string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "run",
		"--rm",
		"-i",
		"--network", "none",
		"--memory", "256m",
		"--cpus", "1",
		s.image(),
		"python", "-c", code,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Include stderr in the output so the agent sees the error.
		combined := strings.TrimSpace(stdout.String() + "\n" + stderr.String())
		return combined, fmt.Errorf("docker exec: %w", err)
	}

	return strings.TrimSpace(stdout.String()), nil
}

// Close is a no-op for the basic Docker sandbox since each Exec call
// uses a fresh container. It exists to satisfy the yac.Sandbox interface.
func (s *Sandbox) Close() error {
	return nil
}
