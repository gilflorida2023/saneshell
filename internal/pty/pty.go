package pty

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

type Manager struct {
	mu sync.Mutex
}

func NewManager() *Manager {
	return &Manager{}
}

func Spawn(cmd string, args []string, env []string) (*exec.Cmd, error) {
	c := exec.Command(cmd, args...)
	c.Env = env
	c.SysProcAttr = &syscall.SysProcAttr{
		Setctty: true,
		Setsid:  true,
	}
	if err := c.Start(); err != nil {
		return nil, err
	}
	return c, nil
}

func WaitWithTimeout(c *exec.Cmd, timeout time.Duration) int {
	done := make(chan struct{})
	go func() {
		c.Wait()
		close(done)
	}()
	select {
	case <-done:
		if c.ProcessState != nil {
			return c.ProcessState.ExitCode()
		}
		return 0
	case <-time.After(timeout):
		c.Process.Kill()
		return -1
	}
}

func ExecPipe(shell, cmd string) int {
	c := exec.Command(shell, "-c", cmd)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return -1
	}
	return 0
}

func ExecPTY(shell, cmd string) int {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return ExecPipe(shell, cmd)
	}

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return ExecPipe(shell, cmd)
	}
	defer term.Restore(fd, oldState)

	// Set stdin to non-blocking so the reader goroutine can be
	// interrupted when the command finishes, avoiding the need
	// for an extra keystroke to unblock wg.Wait().
	syscall.SetNonblock(fd, true)
	defer syscall.SetNonblock(fd, false)

	closeOnce := &sync.Once{}
	ptmxClosed := make(chan struct{})

	c := exec.Command(shell, "-c", cmd)
	ptmx, err := pty.Start(c)
	if err != nil {
		term.Restore(fd, oldState)
		return ExecPipe(shell, cmd)
	}

	closePTMX := func() {
		closeOnce.Do(func() {
			ptmx.Close()
			close(ptmxClosed)
		})
	}
	defer closePTMX()

	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, syscall.SIGWINCH)
	defer signal.Stop(sigch)

	updateWinSize := func() {
		ws, err := pty.GetsizeFull(os.Stdin)
		if err != nil {
			return
		}
		pty.Setsize(ptmx, ws)
	}
	updateWinSize()

	go func() {
		for {
			select {
			case <-sigch:
				updateWinSize()
			case <-ptmxClosed:
				return
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			select {
			case <-ptmxClosed:
				return
			default:
			}
			n, readErr := syscall.Read(fd, buf)
			if readErr != nil {
				if readErr == syscall.EAGAIN || readErr == syscall.EWOULDBLOCK {
					time.Sleep(20 * time.Millisecond)
					continue
				}
				return
			}
			if n == 0 {
				return
			}
			if _, writeErr := ptmx.Write(buf[:n]); writeErr != nil {
				return
			}
		}
	}()

	go func() {
		io.Copy(os.Stdout, ptmx)
		wg.Done()
	}()

	c.Wait()
	closePTMX()
	wg.Wait()

	if c.ProcessState != nil {
		return c.ProcessState.ExitCode()
	}
	return 0
}
