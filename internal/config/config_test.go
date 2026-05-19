package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validYAML = `
server:
  port: 9090
  workspace: "/tmp/workspace"

gitea:
  url: "http://gitea.example.com"
  token: "test-token"

servers:
  dev-server:
    host: "192.168.1.10"
    port: 22
    user: "deploy"
    deploy_path: "/opt/apps"
  sit-server:
    host: "192.168.1.20"
    port: 22
    user: "deploy"
    deploy_path: "/opt/apps"

environments:
  dev:
    label: "开发环境"
  sit:
    label: "集成测试环境"

projects:
  my-project:
    label: "我的项目"
    sub_projects:
      default:
        label: "默认"
        env_overrides:
          dev:
            build_cmd: "npm run build:dev"
            server: "dev-server"
          sit:
            build_cmd: "npm run build:sit"
            server: "sit-server"
`

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	err := os.WriteFile(path, []byte(content), 0644)
	require.NoError(t, err)
	return path
}

func TestLoad_ValidConfig(t *testing.T) {
	path := writeTestConfig(t, validYAML)
	cfg, err := Load(path)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, "/tmp/workspace", cfg.Server.Workspace)
	assert.Equal(t, "http://gitea.example.com", cfg.Gitea.URL)
	assert.Equal(t, "test-token", cfg.Gitea.Token)
	assert.Len(t, cfg.Environments, 2)
	assert.Len(t, cfg.Projects, 1)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file not found")
}

func TestLoad_MalformedYAML(t *testing.T) {
	path := writeTestConfig(t, `{{{invalid yaml:::`)
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid YAML format")
}

func TestLoad_MissingGiteaURL(t *testing.T) {
	yaml := `
gitea:
  url: ""
  token: "some-token"
environments:
  dev:
    label: "dev"
    server:
      host: "localhost"
`
	path := writeTestConfig(t, yaml)
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitea.url must not be empty")
}

func TestLoad_MissingGiteaToken(t *testing.T) {
	yaml := `
gitea:
  url: "http://gitea.example.com"
  token: ""
environments:
  dev:
    label: "dev"
    server:
      host: "localhost"
`
	path := writeTestConfig(t, yaml)
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gitea.token must not be empty")
}

func TestLoad_NoEnvironments(t *testing.T) {
	yaml := `
gitea:
  url: "http://gitea.example.com"
  token: "some-token"
environments:
`
	path := writeTestConfig(t, yaml)
	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one environment must be defined")
}

func TestLoad_DefaultServerPort(t *testing.T) {
	yaml := `
gitea:
  url: "http://gitea.example.com"
  token: "some-token"
environments:
  dev:
    label: "dev"
    server:
      host: "localhost"
`
	path := writeTestConfig(t, yaml)
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "./workspace", cfg.Server.Workspace)
}

func TestGetGiteaConfig(t *testing.T) {
	path := writeTestConfig(t, validYAML)
	cfg, err := Load(path)
	require.NoError(t, err)

	gitea := cfg.GetGiteaConfig()
	assert.Equal(t, "http://gitea.example.com", gitea.URL)
	assert.Equal(t, "test-token", gitea.Token)
}

func TestGetServerConfigForSubProject_Exists(t *testing.T) {
	path := writeTestConfig(t, validYAML)
	cfg, err := Load(path)
	require.NoError(t, err)

	srv, err := cfg.GetServerConfigForSubProject("my-project", "default", "dev")
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.10", srv.Host)
	assert.Equal(t, 22, srv.Port)
	assert.Equal(t, "deploy", srv.User)
	assert.Equal(t, "/opt/apps", srv.DeployPath)
}

func TestGetServerConfigForSubProject_NotFound(t *testing.T) {
	path := writeTestConfig(t, validYAML)
	cfg, err := Load(path)
	require.NoError(t, err)

	_, err = cfg.GetServerConfigForSubProject("my-project", "default", "staging")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `environment "staging" not configured`)
}

func TestGetProjectConfig_Exists(t *testing.T) {
	path := writeTestConfig(t, validYAML)
	cfg, err := Load(path)
	require.NoError(t, err)

	proj, err := cfg.GetProjectConfig("my-project")
	require.NoError(t, err)
	assert.Equal(t, "我的项目", proj.Label)
	assert.Len(t, proj.SubProjects, 1)
}

func TestGetProjectConfig_NotFound(t *testing.T) {
	path := writeTestConfig(t, validYAML)
	cfg, err := Load(path)
	require.NoError(t, err)

	_, err = cfg.GetProjectConfig("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `project "nonexistent" not found`)
}
