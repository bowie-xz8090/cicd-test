package config

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// AppConfig is the top-level application configuration.
type AppConfig struct {
	Server       ServerAppConfig          `yaml:"server" json:"server"`
	Gitea        GiteaConfig              `yaml:"gitea" json:"gitea"`
	Servers      map[string]ServerConfig  `yaml:"servers" json:"servers"`
	Environments map[string]EnvConfig     `yaml:"environments" json:"environments"`
	Projects     map[string]ProjectConfig `yaml:"projects" json:"projects"`
	AdminToken   string                   `yaml:"admin_token" json:"-"`
}

// ServerAppConfig holds the server listening configuration.
type ServerAppConfig struct {
	Port      int    `yaml:"port" json:"port"`
	Workspace string `yaml:"workspace" json:"workspace"`
	Title     string `yaml:"title" json:"title"`
	BasePath  string `yaml:"base_path" json:"base_path"`
}

// GiteaConfig holds Gitea connection information.
type GiteaConfig struct {
	URL      string `yaml:"url" json:"url"`
	Username string `yaml:"username" json:"username"`
	Token    string `yaml:"token" json:"token"`
}

// ServerConfig holds SSH connection details for a target server.
type ServerConfig struct {
	Host       string `yaml:"host" json:"host"`
	Port       int    `yaml:"port" json:"port"`
	User       string `yaml:"user" json:"user"`
	Password   string `yaml:"password" json:"password"`
	DeployPath string `yaml:"deploy_path" json:"deploy_path"`
}

// EnvConfig holds the display configuration for a deployment environment.
type EnvConfig struct {
	Label    string   `yaml:"label" json:"label"`
	Disabled bool     `yaml:"disabled" json:"disabled"`
	Links    EnvLinks `yaml:"links" json:"links"`
	Password string   `yaml:"password" json:"-"`
}

// EnvLinks holds the access URLs for an environment.
type EnvLinks struct {
	UserURL  string    `yaml:"user_url" json:"user_url"`
	AdminURL string    `yaml:"admin_url" json:"admin_url"`
	Extra    []EnvLink `yaml:"extra" json:"extra"`
}

// EnvLink represents a custom link entry.
type EnvLink struct {
	Label string `yaml:"label" json:"label"`
	URL   string `yaml:"url" json:"url"`
}

// ProjectConfig holds build and deploy configuration for a project.
type ProjectConfig struct {
	Label       string                      `yaml:"label" json:"label"`
	SubProjects map[string]SubProjectConfig `yaml:"sub_projects" json:"sub_projects"`
}

// SubProjectConfig holds configuration for a sub-project (deployment target).
type SubProjectConfig struct {
	Label        string                           `yaml:"label" json:"label"`
	EnvOverrides map[string]SubProjectEnvOverride `yaml:"env_overrides" json:"env_overrides"`
}

// SubProjectEnvOverride holds per-environment config for a sub-project.
// Server is a reference key to the top-level servers map.
type SubProjectEnvOverride struct {
	BuildCmd     string           `yaml:"build_cmd" json:"build_cmd"`
	BuildOutput  string           `yaml:"build_output" json:"build_output"`
	DeployScript string           `yaml:"deploy_script" json:"deploy_script"`
	RenameTo     string           `yaml:"rename_to" json:"rename_to"`
	Artifacts    []ArtifactConfig `yaml:"artifacts" json:"artifacts"`
	Server       string           `yaml:"server" json:"server"`
	Disabled     *bool            `yaml:"disabled" json:"disabled"`
}

// ArtifactConfig holds configuration for a single artifact in multi-artifact projects.
type ArtifactConfig struct {
	BuildOutput  string `yaml:"build_output" json:"build_output"`
	DeployScript string `yaml:"deploy_script" json:"deploy_script"`
	RenameTo     string `yaml:"rename_to" json:"rename_to"`
}

// Manager provides thread-safe access to the application configuration
// and supports dynamic reloading from disk.
type Manager struct {
	mu       sync.RWMutex
	cfg      *AppConfig
	filePath string
}

// NewManager creates a new config Manager by loading the config from the given path.
func NewManager(path string) (*Manager, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	return &Manager{
		cfg:      cfg,
		filePath: path,
	}, nil
}

// Get returns a copy of the current configuration (thread-safe).
func (m *Manager) Get() *AppConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// GetFilePath returns the path to the config file.
func (m *Manager) GetFilePath() string {
	return m.filePath
}

// Reload re-reads the config file from disk and replaces the in-memory config.
func (m *Manager) Reload() (*AppConfig, error) {
	cfg, err := Load(m.filePath)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.cfg = cfg
	m.mu.Unlock()
	return cfg, nil
}

// Update replaces the in-memory config with the provided one and persists it to disk.
func (m *Manager) Update(newCfg *AppConfig) error {
	if newCfg.Gitea.URL == "" {
		return fmt.Errorf("config validation error: gitea.url must not be empty")
	}
	if newCfg.Gitea.Token == "" {
		return fmt.Errorf("config validation error: gitea.token must not be empty")
	}
	if len(newCfg.Environments) == 0 {
		return fmt.Errorf("config validation error: at least one environment must be defined")
	}

	if newCfg.Server.Port == 0 {
		newCfg.Server.Port = 8080
	}
	if newCfg.Server.Workspace == "" {
		newCfg.Server.Workspace = "./workspace"
	}

	data, err := yaml.Marshal(newCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(m.filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	m.mu.Lock()
	m.cfg = newCfg
	m.mu.Unlock()

	return nil
}

// Load reads and parses a YAML configuration file from the given path.
func Load(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: invalid YAML format: %w", path, err)
	}

	if cfg.Gitea.URL == "" {
		return nil, fmt.Errorf("config validation error: gitea.url must not be empty")
	}
	if cfg.Gitea.Token == "" {
		return nil, fmt.Errorf("config validation error: gitea.token must not be empty")
	}
	if len(cfg.Environments) == 0 {
		return nil, fmt.Errorf("config validation error: at least one environment must be defined")
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Server.Workspace == "" {
		cfg.Server.Workspace = "./workspace"
	}

	return &cfg, nil
}

// GetGiteaConfig returns the Gitea configuration.
func (c *AppConfig) GetGiteaConfig() GiteaConfig {
	return c.Gitea
}

// ResolveServer looks up a server by key from the top-level servers map.
func (c *AppConfig) ResolveServer(serverKey string) (ServerConfig, error) {
	if serverKey == "" {
		return ServerConfig{}, fmt.Errorf("server key is empty")
	}
	srv, ok := c.Servers[serverKey]
	if !ok {
		return ServerConfig{}, fmt.Errorf("server %q not found in servers config", serverKey)
	}
	if srv.Host == "" {
		return ServerConfig{}, fmt.Errorf("server %q has no host configured", serverKey)
	}
	return srv, nil
}

// GetServerConfigForSubProject returns the server configuration for a sub-project in a given environment.
func (c *AppConfig) GetServerConfigForSubProject(project, subProject, env string) (ServerConfig, error) {
	projCfg, ok := c.Projects[project]
	if !ok {
		return ServerConfig{}, fmt.Errorf("project %q not found in config", project)
	}

	subProjCfg, ok := projCfg.SubProjects[subProject]
	if !ok {
		return ServerConfig{}, fmt.Errorf("sub-project %q not found in project %q", subProject, project)
	}

	envOverride, ok := subProjCfg.EnvOverrides[env]
	if !ok {
		return ServerConfig{}, fmt.Errorf("environment %q not configured for sub-project %q in project %q", env, subProject, project)
	}

	return c.ResolveServer(envOverride.Server)
}

// GetProjectConfig returns the project configuration for the given project name.
func (c *AppConfig) GetProjectConfig(project string) (ProjectConfig, error) {
	projCfg, ok := c.Projects[project]
	if !ok {
		return ProjectConfig{}, fmt.Errorf("project %q not found in config", project)
	}
	return projCfg, nil
}

// GetSubProjectConfigForEnv returns the sub-project configuration for a specific environment.
func (c *AppConfig) GetSubProjectConfigForEnv(project, subProject, env string) (SubProjectEnvOverride, error) {
	projCfg, ok := c.Projects[project]
	if !ok {
		return SubProjectEnvOverride{}, fmt.Errorf("project %q not found in config", project)
	}

	subProjCfg, ok := projCfg.SubProjects[subProject]
	if !ok {
		return SubProjectEnvOverride{}, fmt.Errorf("sub-project %q not found in project %q", subProject, project)
	}

	envOverride, ok := subProjCfg.EnvOverrides[env]
	if !ok {
		return SubProjectEnvOverride{}, fmt.Errorf("environment %q not configured for sub-project %q in project %q", env, subProject, project)
	}

	return envOverride, nil
}

// GetSubProjects returns the list of sub-project keys and labels for a project.
// Returns nil if the project has no sub-projects.
func (c *AppConfig) GetSubProjects(project string) map[string]SubProjectConfig {
	projCfg, ok := c.Projects[project]
	if !ok {
		return nil
	}
	if len(projCfg.SubProjects) == 0 {
		return nil
	}
	return projCfg.SubProjects
}
