package installer

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/fslongjin/liteboxd/tools/liteboxd-installer/internal/sshx"
)

func (i *Installer) deployLiteBoxd() error {
	workspace, cleanup, err := i.buildDeployWorkspace()
	if err != nil {
		return err
	}
	defer cleanup()

	if i.opts.DryRun {
		i.logf("dry-run deploy workspace=%s", workspace)
		return nil
	}

	master, err := sshx.Dial(i.masterTarget(), i.sshTimeout, i.commandTimeout)
	if err != nil {
		return fmt.Errorf("connect master for deploy: %w", err)
	}
	defer master.Close()

	remoteDir := fmt.Sprintf("/tmp/liteboxd-installer-%d", time.Now().Unix())
	tarball, err := buildTarGz(workspace)
	if err != nil {
		return err
	}

	unpackCmd := fmt.Sprintf("mkdir -p %s && tar xzf - -C %s", shellQuote(remoteDir), shellQuote(remoteDir))
	if _, err := master.RunWithInput(unpackCmd, false, bytes.NewReader(tarball)); err != nil {
		return fmt.Errorf("upload workspace: %w", err)
	}

	if _, err := master.Run(fmt.Sprintf("KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl apply -k %s/system-overlay", shellQuote(remoteDir)), true); err != nil {
		return fmt.Errorf("apply system overlay: %w", err)
	}
	if i.cfg.DeploySandbox() {
		if _, err := master.Run(fmt.Sprintf("KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl apply -k %s/sandbox-overlay", shellQuote(remoteDir)), true); err != nil {
			return fmt.Errorf("apply sandbox overlay: %w", err)
		}
	}
	if _, err := master.Run(fmt.Sprintf("rm -rf %s", shellQuote(remoteDir)), false); err != nil {
		i.logf("warn: cleanup remote workspace failed: %v", err)
	}
	return nil
}

func (i *Installer) rolloutCheck() error {
	ns := i.cfg.LiteBoxd.NamespaceSystem
	for _, deploy := range []string{"liteboxd-api", "liteboxd-gateway", "liteboxd-web"} {
		cmd := fmt.Sprintf("KUBECONFIG=/etc/rancher/k3s/k3s.yaml kubectl -n %s rollout status deploy/%s --timeout=600s", shellQuote(ns), deploy)
		if _, err := i.runMaster(cmd, true); err != nil {
			return err
		}
	}
	return nil
}

func (i *Installer) runMaster(command string, useSudo bool) (string, error) {
	if i.opts.DryRun {
		i.logf("dry-run master command: %s", command)
		return "", nil
	}
	start := time.Now()
	i.detailf("target=master host=%s sudo=%t command=%s", i.cfg.Cluster.Master.Host, useSudo, sanitizeCommand(command))
	client, err := sshx.Dial(i.masterTarget(), i.sshTimeout, i.commandTimeout)
	if err != nil {
		i.detailf("target=master dial_error=%v", err)
		return "", fmt.Errorf("connect master: %w", err)
	}
	defer client.Close()
	res, err := client.Run(command, useSudo)
	stdout := sanitizeOutput(command, res.Stdout)
	stderr := sanitizeOutput(command, res.Stderr)
	if stdout != "" {
		i.detailf("target=master stdout=%s", stdout)
	}
	if stderr != "" {
		i.detailf("target=master stderr=%s", stderr)
	}
	i.detailf("target=master duration=%s success=%t", time.Since(start), err == nil)
	if err != nil {
		if res.Stderr != "" {
			return "", fmt.Errorf("%w: %s", err, res.Stderr)
		}
		return "", err
	}
	return res.Stdout, nil
}

func (i *Installer) masterTarget() sshx.Target {
	m := i.cfg.Cluster.Master
	return sshx.Target{
		Host:         m.Host,
		Port:         m.Port,
		User:         m.User,
		Password:     m.Password,
		Sudo:         m.Sudo,
		SudoPassword: m.SudoPassword,
	}
}

func shellQuote(v string) string {
	if v == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
}

var (
	reK3STokenWithSpace = regexp.MustCompile(`K3S_TOKEN=[^\s]+`)
	reK3STokenQuoted    = regexp.MustCompile(`K3S_TOKEN='[^']*'`)
)

func sanitizeCommand(command string) string {
	s := reK3STokenQuoted.ReplaceAllString(command, "K3S_TOKEN='***'")
	s = reK3STokenWithSpace.ReplaceAllString(s, "K3S_TOKEN=***")
	return s
}

func sanitizeOutput(command, out string) string {
	if out == "" {
		return ""
	}
	if strings.Contains(command, "node-token") || strings.Contains(command, "K3S_TOKEN") {
		return "[REDACTED]"
	}
	return out
}
