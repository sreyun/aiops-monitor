//go:build !linux && !darwin

package main

// newContainerExecPTY is unavailable on this platform (no Unix PTY).
func newContainerExecPTY(cli, containerID, shell string, cols, rows int) termShell {
	return nil
}
