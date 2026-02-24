package sshx

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Target struct {
	Host         string
	Port         int
	User         string
	Password     string
	Sudo         bool
	SudoPassword string
}

type Client struct {
	target         Target
	client         *ssh.Client
	parent         *Client
	commandTimeout time.Duration
}

type Result struct {
	Stdout string
	Stderr string
}

func Dial(target Target, timeout time.Duration, commandTimeout time.Duration) (*Client, error) {
	addr := fmt.Sprintf("%s:%d", target.Host, target.Port)
	conf := &ssh.ClientConfig{
		User:            target.User,
		Auth:            []ssh.AuthMethod{ssh.Password(target.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	nc, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	c, chans, reqs, err := ssh.NewClientConn(nc, addr, conf)
	if err != nil {
		_ = nc.Close()
		return nil, fmt.Errorf("ssh handshake %s: %w", addr, err)
	}

	return &Client{
		target:         target,
		client:         ssh.NewClient(c, chans, reqs),
		commandTimeout: commandTimeout,
	}, nil
}

func DialVia(target Target, bastion Target, timeout time.Duration, commandTimeout time.Duration) (*Client, error) {
	bastionClient, err := Dial(bastion, timeout, commandTimeout)
	if err != nil {
		return nil, fmt.Errorf("dial bastion %s:%d: %w", bastion.Host, bastion.Port, err)
	}

	addr := fmt.Sprintf("%s:%d", target.Host, target.Port)
	targetConn, err := bastionClient.client.Dial("tcp", addr)
	if err != nil {
		_ = bastionClient.Close()
		return nil, fmt.Errorf("dial target via bastion %s: %w", addr, err)
	}

	conf := &ssh.ClientConfig{
		User:            target.User,
		Auth:            []ssh.AuthMethod{ssh.Password(target.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}
	c, chans, reqs, err := ssh.NewClientConn(targetConn, addr, conf)
	if err != nil {
		_ = targetConn.Close()
		_ = bastionClient.Close()
		return nil, fmt.Errorf("ssh handshake via bastion %s: %w", addr, err)
	}

	return &Client{
		target:         target,
		client:         ssh.NewClient(c, chans, reqs),
		parent:         bastionClient,
		commandTimeout: commandTimeout,
	}, nil
}

func (c *Client) Close() error {
	var firstErr error
	if c.client != nil {
		if err := c.client.Close(); err != nil {
			firstErr = err
		}
	}
	if c.parent != nil {
		if err := c.parent.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (c *Client) Run(command string, useSudo bool) (Result, error) {
	return c.run(command, useSudo, nil)
}

func (c *Client) RunWithInput(command string, useSudo bool, in io.Reader) (Result, error) {
	return c.run(command, useSudo, in)
}

func (c *Client) run(command string, useSudo bool, in io.Reader) (Result, error) {
	session, err := c.client.NewSession()
	if err != nil {
		return Result{}, fmt.Errorf("create ssh session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr
	if in != nil {
		session.Stdin = in
	}

	wrapped := c.wrap(command, useSudo)
	if err := session.Start(wrapped); err != nil {
		return Result{}, fmt.Errorf("start command: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- session.Wait()
	}()

	select {
	case err := <-done:
		res := Result{Stdout: strings.TrimSpace(stdout.String()), Stderr: strings.TrimSpace(stderr.String())}
		if err != nil {
			return res, fmt.Errorf("command failed: %w", err)
		}
		return res, nil
	case <-time.After(c.commandTimeout):
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		return Result{Stdout: strings.TrimSpace(stdout.String()), Stderr: strings.TrimSpace(stderr.String())}, errors.New("command timed out")
	}
}

func (c *Client) wrap(command string, useSudo bool) string {
	if !useSudo || !c.target.Sudo {
		return "bash -lc " + shellQuote(command)
	}

	password := c.target.SudoPassword
	if password == "" {
		password = c.target.Password
	}
	return fmt.Sprintf("printf '%%s\\n' %s | sudo -S -p '' bash -lc %s", shellQuote(password), shellQuote(command))
}

func shellQuote(v string) string {
	if v == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
}
