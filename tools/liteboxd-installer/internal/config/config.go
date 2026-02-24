package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config defines installer configuration.
type Config struct {
	Cluster  ClusterConfig  `yaml:"cluster"`
	Network  NetworkConfig  `yaml:"network"`
	LiteBoxd LiteBoxdConfig `yaml:"liteboxd"`
	Runtime  RuntimeConfig  `yaml:"runtime"`
}

type ClusterConfig struct {
	Name       string           `yaml:"name"`
	K3sInstall K3sInstallConfig `yaml:"k3sInstall"`
	Registries RegistriesConfig `yaml:"registries"`
	Master     NodeConfig       `yaml:"master"`
	Agents     []NodeConfig     `yaml:"agents"`
}

type K3sInstallConfig struct {
	ScriptURL string `yaml:"scriptURL"`
	Mirror    string `yaml:"mirror"`
}

type RegistriesConfig struct {
	Mirrors map[string]RegistryMirror `yaml:"mirrors"`
}

type RegistryMirror struct {
	Endpoint []string `yaml:"endpoint"`
}

type NodeConfig struct {
	Host         string    `yaml:"host"`
	NodeIP       string    `yaml:"nodeIP"`
	Port         int       `yaml:"port"`
	User         string    `yaml:"user"`
	Password     string    `yaml:"password"`
	Sudo         bool      `yaml:"sudo"`
	SudoPassword string    `yaml:"sudoPassword"`
	K3s          K3sConfig `yaml:"k3s"`
}

type K3sConfig struct {
	Version     string   `yaml:"version"`
	TLSSAN      []string `yaml:"tlsSAN"`
	InstallArgs []string `yaml:"installArgs"`
}

type NetworkConfig struct {
	CNI    string       `yaml:"cni"`
	Cilium CiliumConfig `yaml:"cilium"`
}

type CiliumConfig struct {
	Version              string `yaml:"version"`
	CLIVersion           string `yaml:"cliVersion"`
	CLIDownloadBaseURL   string `yaml:"cliDownloadBaseURL"`
	PodCIDR              string `yaml:"podCIDR"`
	KubeProxyReplacement bool   `yaml:"kubeProxyReplacement"`
	EnableEgressGateway  bool   `yaml:"enableEgressGateway"`
}

type LiteBoxdConfig struct {
	NamespaceSystem        string      `yaml:"namespaceSystem"`
	NamespaceSandbox       string      `yaml:"namespaceSandbox"`
	IngressHost            string      `yaml:"ingressHost"`
	GatewayURL             string      `yaml:"gatewayURL"`
	ConfigDir              string      `yaml:"configDir"`
	Images                 ImageConfig `yaml:"images"`
	DeploySandboxResources *bool       `yaml:"deploySandboxResources"`
}

type ImageConfig struct {
	API     string `yaml:"api"`
	Gateway string `yaml:"gateway"`
	Web     string `yaml:"web"`
}

type RuntimeConfig struct {
	Parallelism           int  `yaml:"parallelism"`
	SSHTimeoutSeconds     int  `yaml:"sshTimeoutSeconds"`
	CommandTimeoutSeconds int  `yaml:"commandTimeoutSeconds"`
	RemoveAbsentAgents    bool `yaml:"removeAbsentAgents"`
	DryRun                bool `yaml:"dryRun"`
}

type LoadOptions struct {
	RequireLiteBoxd bool
}

func Load(path string) (*Config, error) {
	return LoadWithOptions(path, LoadOptions{RequireLiteBoxd: true})
}

func LoadWithOptions(path string, opts LoadOptions) (*Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %q: %w", path, err)
	}

	expandEnvStrings(reflect.ValueOf(&cfg))
	cfg.setDefaults(path)

	if err := cfg.Validate(opts); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) setDefaults(configPath string) {
	if c.Cluster.Name == "" {
		c.Cluster.Name = "liteboxd"
	}
	if c.Cluster.K3sInstall.Mirror == "" {
		c.Cluster.K3sInstall.Mirror = "default"
	}
	if c.Cluster.K3sInstall.ScriptURL == "" {
		if c.Cluster.K3sInstall.Mirror == "cn" {
			c.Cluster.K3sInstall.ScriptURL = "https://rancher-mirror.rancher.cn/k3s/k3s-install.sh"
		} else {
			c.Cluster.K3sInstall.ScriptURL = "https://get.k3s.io"
		}
	}

	if c.Cluster.Master.Port == 0 {
		c.Cluster.Master.Port = 22
	}
	if c.Cluster.Master.NodeIP == "" {
		c.Cluster.Master.NodeIP = c.Cluster.Master.Host
	}
	if c.Cluster.Master.User == "" {
		c.Cluster.Master.User = "root"
	}

	for i := range c.Cluster.Agents {
		if c.Cluster.Agents[i].Port == 0 {
			c.Cluster.Agents[i].Port = 22
		}
		if c.Cluster.Agents[i].NodeIP == "" {
			c.Cluster.Agents[i].NodeIP = c.Cluster.Agents[i].Host
		}
		if c.Cluster.Agents[i].User == "" {
			c.Cluster.Agents[i].User = "root"
		}
	}

	if c.Network.CNI == "" {
		c.Network.CNI = "cilium"
	}
	if c.Network.Cilium.Version == "" {
		c.Network.Cilium.Version = "1.18.6"
	}
	if c.Network.Cilium.CLIVersion == "" {
		c.Network.Cilium.CLIVersion = "v0.19.1"
	}
	if c.Network.Cilium.CLIDownloadBaseURL == "" {
		c.Network.Cilium.CLIDownloadBaseURL = "https://cnb.cool/DragonOS-Community/cilium-cli/-/releases/download"
	}
	if c.Network.Cilium.PodCIDR == "" {
		c.Network.Cilium.PodCIDR = "10.42.0.0/16"
	}

	if c.LiteBoxd.NamespaceSystem == "" {
		c.LiteBoxd.NamespaceSystem = "liteboxd-system"
	}
	if c.LiteBoxd.NamespaceSandbox == "" {
		c.LiteBoxd.NamespaceSandbox = "liteboxd-sandbox"
	}
	if c.LiteBoxd.IngressHost == "" {
		c.LiteBoxd.IngressHost = "liteboxd.local"
	}
	if c.LiteBoxd.ConfigDir != "" && !filepath.IsAbs(c.LiteBoxd.ConfigDir) {
		c.LiteBoxd.ConfigDir = resolveRelativePath(configPath, c.LiteBoxd.ConfigDir)
	}
	if c.LiteBoxd.DeploySandboxResources == nil {
		v := true
		c.LiteBoxd.DeploySandboxResources = &v
	}

	if c.Runtime.Parallelism <= 0 {
		c.Runtime.Parallelism = 5
	}
	if c.Runtime.SSHTimeoutSeconds <= 0 {
		c.Runtime.SSHTimeoutSeconds = 15
	}
	if c.Runtime.CommandTimeoutSeconds <= 0 {
		c.Runtime.CommandTimeoutSeconds = 1200
	}
}

func resolveRelativePath(configPath, relPath string) string {
	cwdRelative := filepath.Clean(relPath)
	if _, err := os.Stat(cwdRelative); err == nil {
		return cwdRelative
	}
	cfgDir := filepath.Dir(configPath)
	cfgRelative := filepath.Clean(filepath.Join(cfgDir, relPath))
	if _, err := os.Stat(cfgRelative); err == nil {
		return cfgRelative
	}
	return cwdRelative
}

func (c *Config) Validate(opts LoadOptions) error {
	if c.Cluster.Master.Host == "" {
		return errors.New("cluster.master.host is required")
	}
	if c.Cluster.Master.NodeIP == "" {
		return errors.New("cluster.master.nodeIP is required")
	}
	if c.Cluster.Master.Password == "" {
		return errors.New("cluster.master.password is required (supports ${ENV_VAR})")
	}
	if c.Network.CNI != "cilium" {
		return fmt.Errorf("network.cni must be \"cilium\" in v1, got %q", c.Network.CNI)
	}
	if c.Cluster.K3sInstall.Mirror != "default" && c.Cluster.K3sInstall.Mirror != "cn" {
		return fmt.Errorf("cluster.k3sInstall.mirror must be one of [default, cn], got %q", c.Cluster.K3sInstall.Mirror)
	}
	if c.Cluster.K3sInstall.ScriptURL == "" {
		return errors.New("cluster.k3sInstall.scriptURL is required")
	}
	if c.Cluster.Master.K3s.Version == "" {
		return errors.New("cluster.master.k3s.version is required")
	}
	if c.Cluster.K3sInstall.Mirror == "cn" {
		ok, err := versionLTE(c.Network.Cilium.Version, "1.18.6")
		if err != nil {
			return fmt.Errorf("invalid network.cilium.version %q: %w", c.Network.Cilium.Version, err)
		}
		if !ok {
			return fmt.Errorf("for cn mirror mode, network.cilium.version must be <= 1.18.6, got %q", c.Network.Cilium.Version)
		}
	}
	if opts.RequireLiteBoxd {
		if c.LiteBoxd.ConfigDir == "" {
			return errors.New("liteboxd.configDir is required")
		}
		if c.LiteBoxd.Images.API == "" || c.LiteBoxd.Images.Gateway == "" || c.LiteBoxd.Images.Web == "" {
			return errors.New("liteboxd.images.api/gateway/web are required")
		}
		if _, err := os.Stat(c.LiteBoxd.ConfigDir); err != nil {
			return fmt.Errorf("liteboxd.configDir %q is invalid: %w", c.LiteBoxd.ConfigDir, err)
		}
	}

	seenHosts := map[string]struct{}{c.Cluster.Master.Host: {}}
	seenNodeIPs := map[string]struct{}{c.Cluster.Master.NodeIP: {}}
	for _, a := range c.Cluster.Agents {
		if a.Host == "" {
			return errors.New("cluster.agents[].host is required")
		}
		if a.NodeIP == "" {
			return fmt.Errorf("cluster.agents[%s].nodeIP is required", a.Host)
		}
		if a.Password == "" {
			return fmt.Errorf("cluster.agents[%s].password is required", a.Host)
		}
		if _, ok := seenHosts[a.Host]; ok {
			return fmt.Errorf("duplicate host found in cluster config: %s", a.Host)
		}
		seenHosts[a.Host] = struct{}{}
		if _, ok := seenNodeIPs[a.NodeIP]; ok {
			return fmt.Errorf("duplicate nodeIP found in cluster config: %s", a.NodeIP)
		}
		seenNodeIPs[a.NodeIP] = struct{}{}
	}
	return nil
}

func (c *Config) DeploySandbox() bool {
	return c.LiteBoxd.DeploySandboxResources != nil && *c.LiteBoxd.DeploySandboxResources
}

func (c *Config) DesiredAgentHosts() []string {
	hosts := make([]string, 0, len(c.Cluster.Agents))
	for _, a := range c.Cluster.Agents {
		hosts = append(hosts, a.Host)
	}
	sort.Strings(hosts)
	return hosts
}

func (c *Config) DesiredAgentNodeIPs() []string {
	ips := make([]string, 0, len(c.Cluster.Agents))
	for _, a := range c.Cluster.Agents {
		ips = append(ips, a.NodeIP)
	}
	sort.Strings(ips)
	return ips
}

func expandEnvStrings(v reflect.Value) {
	if !v.IsValid() {
		return
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return
		}
		expandEnvStrings(v.Elem())
		return
	}

	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			expandEnvStrings(v.Field(i))
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			expandEnvStrings(v.Index(i))
		}
	case reflect.String:
		if v.CanSet() {
			v.SetString(strings.TrimSpace(os.ExpandEnv(v.String())))
		}
	}
}

func versionLTE(actual, max string) (bool, error) {
	a, err := parseVersion(actual)
	if err != nil {
		return false, err
	}
	m, err := parseVersion(max)
	if err != nil {
		return false, err
	}
	for i := 0; i < 3; i++ {
		if a[i] < m[i] {
			return true, nil
		}
		if a[i] > m[i] {
			return false, nil
		}
	}
	return true, nil
}

func parseVersion(v string) ([3]int, error) {
	var out [3]int
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		return out, fmt.Errorf("version should be semver-like, got %q", v)
	}
	for i := 0; i < 3; i++ {
		p := parts[i]
		for idx, ch := range p {
			if ch < '0' || ch > '9' {
				p = p[:idx]
				break
			}
		}
		if p == "" {
			return out, fmt.Errorf("invalid numeric part in version %q", v)
		}
		n := 0
		for _, ch := range p {
			n = n*10 + int(ch-'0')
		}
		out[i] = n
	}
	return out, nil
}
