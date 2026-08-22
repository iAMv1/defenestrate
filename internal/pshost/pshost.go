// Package pshost owns the persistent PowerShell engine process. Shell-first
// architecture: Windows-native work (tree measurement, recycle, system ops)
// executes inside ONE long-lived powershell.exe instead of paying a ~450ms
// cold spawn per operation. Go stays the safety kernel; this is the engine.
package pshost

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

type Session struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Reader
	seq int
}

var (
	defaultSession *Session
	startErr       error
	once           sync.Once
)

// Default lazily starts the shared engine session.
func Default() (*Session, error) {
	once.Do(func() { defaultSession, startErr = Start() })
	return defaultSession, startErr
}

// Start launches powershell reading commands from stdin (- as -Command arg).
func Start() (*Session, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", "-")
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// Drain stderr so a chatty script can never block on a full pipe.
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	s := &Session{cmd: cmd, in: in, out: bufio.NewReaderSize(out, 1<<20)}
	return s, nil
}

// Run executes one script block inside the shared engine and returns its
// stdout. Serialized: PowerShell stdin is a conversation, not parallel.
// The marker line is emitted by us after the script so partial output (and
// scripts that print nothing) are handled uniformly.
func (s *Session) Run(script string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	id := fmt.Sprintf("__PSDONE_%d_%d__", s.cmd.Process.Pid, s.seq)
	payload := script + "\nWrite-Output '" + id + "'\n"
	if _, err := io.WriteString(s.in, payload); err != nil {
		// Engine died; force a fresh one next call.
		s.restartLocked()
		return "", fmt.Errorf("engine write: %w", err)
	}
	var sb strings.Builder
	for {
		line, err := s.out.ReadString('\n')
		if err != nil {
			s.restartLocked()
			return sb.String(), fmt.Errorf("engine read: %w", err)
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == id {
			return sb.String(), nil
		}
		sb.WriteString(trimmed)
		sb.WriteByte('\n')
	}
}

// restartLocked tears down a dead engine so the next Run spawns a fresh one.
func (s *Session) restartLocked() {
	if s.in != nil {
		_ = s.in.Close()
	}
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	ns, err := Start()
	if err == nil {
		s.cmd, s.in, s.out = ns.cmd, ns.in, ns.out
	}
}
