//go:build !windows

package main

import (
	"os/exec"
	"strings"
	"sync/atomic"
	"syscall"
)

// Shared helpers for the Unix (Linux/macOS) daemon supervisors.

func runSvcCmd(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

// setEnvKV replaces or appends KEY=value in an environment slice.
func setEnvKV(env *[]string, key, val string) {
	prefix := key + "="
	for i, kv := range *env {
		if strings.HasPrefix(kv, prefix) {
			(*env)[i] = prefix + val
			return
		}
	}
	*env = append(*env, prefix+val)
}

// bgProc is a supervised child process (a desktop worker). Wait() runs in the
// background to reap the child and flip the alive flag, avoiding zombies.
type bgProc struct {
	cmd  *exec.Cmd
	live atomic.Bool
}

func newBgProc(cmd *exec.Cmd) *bgProc {
	p := &bgProc{cmd: cmd}
	p.live.Store(true)
	go func() {
		_ = cmd.Wait()
		p.live.Store(false)
	}()
	return p
}

func (p *bgProc) alive() bool {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return false
	}
	return p.live.Load()
}

func (p *bgProc) kill() {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	// Kill the whole process group (Setpgid) so ffmpeg/xdotool children die too.
	pid := p.cmd.Process.Pid
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGTERM)
	}
	_ = p.cmd.Process.Kill()
	p.live.Store(false)
}
