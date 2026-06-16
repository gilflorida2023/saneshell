package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/saneshell/saneshell/internal/config"
	"github.com/saneshell/saneshell/internal/editor"
	"github.com/saneshell/saneshell/internal/history"
	"github.com/saneshell/saneshell/internal/ipc"
)

var version = "0.1.0"

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	configPath := flag.String("config", "", "config file path")
	flag.Parse()

	if *showVersion {
		fmt.Printf("saneshell %s\n", version)
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	histStore, err := history.Open(cfg.History.Path)
	if err != nil {
		log.Printf("history: %v (continuing without)", err)
		histStore = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)
	go func() {
		<-sigCh
		cancel()
	}()

	var ipcClient *ipc.Client
	if cfg.Intel.Enabled {
		socketPath := cfg.Intel.SocketPath
		if socketPath == "" {
			socketPath = fmt.Sprintf(ipc.SocketPath, os.Getuid())
		}
		session := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().Unix())
		client, err := ipc.Dial(socketPath, session)
		if err != nil {
			_ = err // silent; user can enable daemon later
		} else {
			ipcClient = client
			defer client.Close()
		}
	}

	promptFunc := func() string {
		user := os.Getenv("USER")
		if user == "" {
			user = "?"
		}
		host, _ := os.Hostname()
		if i := strings.Index(host, "."); i >= 0 {
			host = host[:i]
		}
		cwd, _ := os.Getwd()
		home, _ := os.UserHomeDir()
		if strings.HasPrefix(cwd, home) {
			cwd = "~" + cwd[len(home):]
		}
		p := cfg.Editor.Prompt
		p = strings.ReplaceAll(p, "{{.User}}", user)
		p = strings.ReplaceAll(p, "{{.Host}}", host)
		p = strings.ReplaceAll(p, "{{.CWD}}", cwd)
		return p
	}

	ed := editor.New(promptFunc, cfg.Editor.Mode == "vi")

	fmt.Printf("\n  \033[1;36m\u2699 saneshell\033[0m\n")
	fmt.Printf("  welcome. type \033[33mexit\033[0m or ^D to quit.\n\n")

	for {
		line, err := ed.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if strings.Contains(err.Error(), "interrupt") {
				continue
			}
			break
		}
		if line == "" {
			continue
		}
		if line == "exit" {
			break
		}

		// Handle cd internally
		if strings.HasPrefix(line, "cd ") || line == "cd" || strings.HasPrefix(line, "cd~") {
			handleCd(line)
			continue
		}
		if line == "pushd" || strings.HasPrefix(line, "pushd ") || line == "popd" {
			handleCd(line)
			continue
		}

		start := time.Now()
		rc, _ := editor.RunCmd(line)
		elapsed := time.Since(start)

		_ = elapsed
		_ = rc

		if histStore != nil {
			cwd, _ := os.Getwd()
			histStore.Add(ctx, &history.Entry{
				Timestamp: time.Now(),
				Cmd:       line,
				CWD:       cwd,
				RC:        rc,
				Duration:  elapsed.Milliseconds(),
				Session:   fmt.Sprintf("%d", os.Getpid()),
			})
		}

		if ipcClient != nil && rc != 0 {
			ipcClient.SendAsync(&ipc.BaseMessage{
				Type: ipc.MsgLearn,
				Proto: 1,
				TS:   time.Now().UnixMilli(),
			})
		}
	}

	fmt.Println("  goodbye.")
}

var dirStack []string

func handleCd(raw string) {
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return
	}
	switch parts[0] {
	case "cd":
		target := ""
		if len(parts) == 1 {
			target = os.Getenv("HOME")
			if target == "" {
				target = "/"
			}
		} else {
			target = parts[1]
			if len(parts) > 2 {
				fmt.Fprintf(os.Stderr, "bash: cd: too many arguments\n")
				return
			}
		}
		if target == "-" {
			target = os.Getenv("OLDPWD")
			if target == "" {
				return
			}
			fmt.Println(target)
		}
		if target == "~" || strings.HasPrefix(target, "~/") {
			home := os.Getenv("HOME")
			if target == "~" {
				target = home
			} else {
				target = home + target[1:]
			}
		}
		oldPwd, _ := os.Getwd()
		if err := os.Chdir(target); err != nil {
			fmt.Fprintf(os.Stderr, "bash: cd: %s: %v\n", target, err)
			return
		}
		os.Setenv("OLDPWD", oldPwd)
	case "pushd":
		if len(parts) == 1 && len(dirStack) >= 2 {
			// pushd with no args: rotate top two entries
			dirStack[0], dirStack[1] = dirStack[1], dirStack[0]
			if err := os.Chdir(dirStack[0]); err != nil {
				dirStack[0], dirStack[1] = dirStack[1], dirStack[0]
				fmt.Fprintf(os.Stderr, "bash: pushd: %s: %v\n", dirStack[0], err)
				return
			}
			fmt.Println(strings.Join(append([]string{os.Getenv("HOME")}, dirStack...), " "))
			return
		}
		target := parts[1]
		oldPwd, _ := os.Getwd()
		if err := os.Chdir(target); err != nil {
			fmt.Fprintf(os.Stderr, "bash: pushd: %s: %v\n", target, err)
			return
		}
		dirStack = append([]string{oldPwd}, dirStack...)
		fmt.Println(strings.Join(append([]string{target}, dirStack...), " "))
		os.Setenv("OLDPWD", oldPwd)
	case "popd":
		if len(dirStack) == 0 {
			fmt.Fprintf(os.Stderr, "bash: popd: directory stack empty\n")
			return
		}
		target := dirStack[0]
		dirStack = dirStack[1:]
		if err := os.Chdir(target); err != nil {
			fmt.Fprintf(os.Stderr, "bash: popd: %s: %v\n", target, err)
			return
		}
		os.Setenv("OLDPWD", target)
		if len(dirStack) > 0 {
			fmt.Println(strings.Join(dirStack, " "))
		}
	}
}