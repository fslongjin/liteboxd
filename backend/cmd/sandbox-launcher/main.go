package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

type launcherConfig struct {
	noFile  uint64
	command []string
}

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &syscall.Rlimit{
		Cur: cfg.noFile,
		Max: cfg.noFile,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "setrlimit RLIMIT_NOFILE=%d: %v\n", cfg.noFile, err)
		os.Exit(1)
	}

	bin, err := exec.LookPath(cfg.command[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve command %q: %v\n", cfg.command[0], err)
		os.Exit(1)
	}

	if err := syscall.Exec(bin, cfg.command, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "exec %q: %v\n", bin, err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (*launcherConfig, error) {
	fs := flag.NewFlagSet("sandbox-launcher", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var noFileRaw string
	fs.StringVar(&noFileRaw, "nofile", "", "RLIMIT_NOFILE value")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if noFileRaw == "" {
		return nil, errors.New("missing required --nofile")
	}
	value, err := strconv.ParseUint(noFileRaw, 10, 64)
	if err != nil || value == 0 {
		return nil, fmt.Errorf("invalid --nofile value %q", noFileRaw)
	}
	command := fs.Args()
	if len(command) == 0 {
		return nil, errors.New("missing command after --")
	}
	return &launcherConfig{
		noFile:  value,
		command: command,
	}, nil
}
