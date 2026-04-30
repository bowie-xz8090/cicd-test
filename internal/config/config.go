package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AppConfig is the top-level application configuration.
type AppConfig struct {
	Server       ServerAppConfig          `yaml:"server"`
	Gitea        GiteaConfig              `yaml:"gitea"`
	Environments map[string]EnvConfig     `yaml:"environments"`
	Projects     map[string]ProjectConfig `yaml:"projects"`
}

// ServerAppConfig holds the server listening configuration.
type ServerAppConfig struct {
	Port      int    `yaml:"port"`
	Workspace string `yaml:"workspace"`
}

// GiteaConfig holds Gitea connection information.
type GiteaConfig struct {
	URL   string `yaml:"url"`
	Token string `yaml:"token"`
}

// EnvConfig holds the configuration for a deployment environment.
type EnvConfig struct {
	Label  string       `yaml:"label"`
	Server ServerConfig `yaml:"server"`
}

// ServerConfig holds SSH connection details for a target server.
type ServerConfig struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	User       string `yaml:"user"`
	Password   string `yaml:"password"`
	DeployPath string `yaml:"deploy_path"`
}

// ProjectConfig holds build and deploy configuration for a project.
type ProjectConfig struct {
	BuildCmd     string `yaml:"build_cmd"`
	BuildOutput  string `yaml:"build_output"`
	DeployScript string `yaml:"deploy_script"`
}

// Load reads and parses a YAML configuration file from the given path.
// It validates that required fields are present and returns clear error messages
// for missing files, malformed YAML, or missing required configuration.
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

	// Validate required fields
	if cfg.Gitea.URL == "" {
		return nil, fmt.Errorf("config validation error: gitea.url must not be empty")
	}
	if cfg.Gitea.Token == "" {
		return nil, fmt.Errorf("config validation error: gitea.token must not be empty")
	}
	if len(cfg.Environments) == 0 {
		return nil, fmt.Errorf("config validation error: at least one environment must be defined")
	}

	// Apply defaults
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

// GetServerConfig returns the server configuration for the given environment.
func (c *AppConfig) GetServerConfig(env string) (ServerConfig, error) {
	envCfg, ok := c.Environments[env]
	if !ok {
		return ServerConfig{}, fmt.Errorf("environment %q not found in config", env)
	}
	return envCfg.Server, nil
}

// GetProjectConfig returns the project configuration for the given project name.
func (c *AppConfig) GetProjectConfig(project string) (ProjectConfig, error) {
	projCfg, ok := c.Projects[project]
	if !ok {
		return ProjectConfig{}, fmt.Errorf("project %q not found in config", project)
	}
	return projCfg, nil
}
