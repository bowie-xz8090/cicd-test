package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"auto-deploy-platform/internal/config"
	"auto-deploy-platform/internal/db"
	"auto-deploy-platform/internal/gitea"
	"auto-deploy-platform/internal/task"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockGiteaClient is a test double for gitea.GiteaClient.
type mockGiteaClient struct {
	repos     []gitea.Repository
	branches  []gitea.Branch
	tags      []gitea.Tag
	repoErr   error
	branchErr error
	tagErr    error
}

func (m *mockGiteaClient) ListRepos() ([]gitea.Repository, error) {
	if m.repoErr != nil {
		return nil, m.repoErr
	}
	return m.repos, nil
}

func (m *mockGiteaClient) ListBranches(owner, repo string) ([]gitea.Branch, error) {
	if m.branchErr != nil {
		return nil, m.branchErr
	}
	return m.branches, nil
}

func (m *mockGiteaClient) ListTags(owner, repo string) ([]gitea.Tag, error) {
	if m.tagErr != nil {
		return nil, m.tagErr
	}
	return m.tags, nil
}

// mockTaskManager is a test double for task.TaskManager.
type mockTaskManager struct {
	createTaskFn   func(req task.DeployRequest) (*db.DeployTask, error)
	getStatusFn    func(taskID string) (*task.TaskStatus, error)
	getLogsFn      func(taskID string) (string, error)
	listRecordsFn  func(filter task.RecordFilter) ([]task.DeployRecord, int, error)
	cancelTaskFn   func(taskID string) error
}

func (m *mockTaskManager) CreateTask(req task.DeployRequest) (*db.DeployTask, error) {
	if m.createTaskFn != nil {
		return m.createTaskFn(req)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTaskManager) GetTaskStatus(taskID string) (*task.TaskStatus, error) {
	if m.getStatusFn != nil {
		return m.getStatusFn(taskID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockTaskManager) GetTaskLogs(taskID string) (string, error) {
	if m.getLogsFn != nil {
		return m.getLogsFn(taskID)
	}
	return "", fmt.Errorf("not implemented")
}

func (m *mockTaskManager) ListRecords(filter task.RecordFilter) ([]task.DeployRecord, int, error) {
	if m.listRecordsFn != nil {
		return m.listRecordsFn(filter)
	}
	return nil, 0, fmt.Errorf("not implemented")
}

func (m *mockTaskManager) CancelTask(taskID string) error {
	if m.cancelTaskFn != nil {
		return m.cancelTaskFn(taskID)
	}
	return fmt.Errorf("not implemented")
}

func setupRouter(giteaClient gitea.GiteaClient, cfg *config.AppConfig) *gin.Engine {
	return setupRouterWithTaskMgr(giteaClient, cfg, nil)
}

func setupRouterWithTaskMgr(giteaClient gitea.GiteaClient, cfg *config.AppConfig, taskMgr task.TaskManager) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewHandler(giteaClient, cfg, taskMgr)
	h.RegisterRoutes(r)
	return r
}

func testConfig() *config.AppConfig {
	return &config.AppConfig{
		Servers: map[string]config.ServerConfig{
			"dev-server": {
				Host: "192.168.1.10",
				Port: 22,
				User: "deploy",
			},
		},
		Environments: map[string]config.EnvConfig{
			"dev": {
				Label: "开发环境",
			},
			"sit": {
				Label: "集成测试环境",
			},
			"prod": {
				Label: "生产环境",
			},
		},
		Projects: map[string]config.ProjectConfig{
			"project-a": {
				Label: "项目A",
				SubProjects: map[string]config.SubProjectConfig{
					"default": {
						Label: "默认",
						EnvOverrides: map[string]config.SubProjectEnvOverride{
							"dev": {Server: "dev-server"},
						},
					},
				},
			},
			"project-b": {
				Label: "项目B",
				SubProjects: map[string]config.SubProjectConfig{
					"default": {
						Label: "默认",
						EnvOverrides: map[string]config.SubProjectEnvOverride{
							"dev": {Server: "dev-server"},
						},
					},
				},
			},
		},
	}
}

func TestHealthEndpoint(t *testing.T) {
	r := setupRouter(&mockGiteaClient{}, testConfig())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/health", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp["code"])
	assert.Equal(t, "ok", resp["data"])
}

func TestListProjects_Success(t *testing.T) {
	mock := &mockGiteaClient{
		repos: []gitea.Repository{
			{Owner: "org", Name: "project-a", FullName: "org/project-a", Description: "Project A", CloneURL: "http://gitea.example.com/org/project-a.git"},
			{Owner: "org", Name: "project-b", FullName: "org/project-b", Description: "Project B", CloneURL: "http://gitea.example.com/org/project-b.git"},
		},
	}
	r := setupRouter(mock, testConfig())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/projects", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp["code"])

	data, ok := resp["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 2)

	first := data[0].(map[string]interface{})
	assert.Equal(t, "org", first["owner"])
	assert.Equal(t, "project-a", first["name"])
}

func TestListProjects_GiteaError(t *testing.T) {
	mock := &mockGiteaClient{
		repoErr: assert.AnError,
	}
	r := setupRouter(mock, testConfig())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/projects", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(-1), resp["code"])
	assert.Contains(t, resp["message"], "获取项目列表失败")
}

func TestListBranches_Success(t *testing.T) {
	mock := &mockGiteaClient{
		branches: []gitea.Branch{
			{Name: "main", CommitID: "abc123"},
			{Name: "develop", CommitID: "def456"},
		},
	}
	r := setupRouter(mock, testConfig())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/projects/org/my-repo/branches", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp["code"])

	data, ok := resp["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 2)

	first := data[0].(map[string]interface{})
	assert.Equal(t, "main", first["name"])
	assert.Equal(t, "abc123", first["commit_id"])
}

func TestListBranches_GiteaError(t *testing.T) {
	mock := &mockGiteaClient{
		branchErr: assert.AnError,
	}
	r := setupRouter(mock, testConfig())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/projects/org/my-repo/branches", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(-1), resp["code"])
	assert.Contains(t, resp["message"], "获取分支列表失败")
}

func TestListEnvironments(t *testing.T) {
	r := setupRouter(&mockGiteaClient{}, testConfig())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/environments", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp["code"])

	data, ok := resp["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 3)

	// Collect all keys and labels from the response
	keys := make(map[string]string)
	for _, item := range data {
		env := item.(map[string]interface{})
		key := env["key"].(string)
		label := env["label"].(string)
		keys[key] = label
	}

	assert.Equal(t, "开发环境", keys["dev"])
	assert.Equal(t, "集成测试环境", keys["sit"])
	assert.Equal(t, "生产环境", keys["prod"])
}

func TestAllEndpoints_NoAuth(t *testing.T) {
	mock := &mockGiteaClient{
		repos:    []gitea.Repository{},
		branches: []gitea.Branch{},
	}
	taskMgr := &mockTaskManager{
		listRecordsFn: func(filter task.RecordFilter) ([]task.DeployRecord, int, error) {
			return []task.DeployRecord{}, 0, nil
		},
	}
	r := setupRouterWithTaskMgr(mock, testConfig(), taskMgr)

	// All endpoints should be accessible without any auth headers
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/health"},
		{http.MethodGet, "/api/projects"},
		{http.MethodGet, "/api/projects/org/repo/branches"},
		{http.MethodGet, "/api/environments"},
		{http.MethodGet, "/api/deploy/records"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			// No Authorization header set
			r.ServeHTTP(w, req)

			// Should not return 401 or 403
			assert.NotEqual(t, http.StatusUnauthorized, w.Code)
			assert.NotEqual(t, http.StatusForbidden, w.Code)
		})
	}
}

func TestListProjects_EmptyList(t *testing.T) {
	mock := &mockGiteaClient{
		repos: []gitea.Repository{},
	}
	r := setupRouter(mock, testConfig())

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/projects", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp["code"])

	data, ok := resp["data"].([]interface{})
	require.True(t, ok)
	assert.Len(t, data, 0)
}

// --- Deploy endpoint tests ---

func TestHandleDeploy_Success(t *testing.T) {
	now := time.Now().UTC()
	taskMgr := &mockTaskManager{
		createTaskFn: func(req task.DeployRequest) (*db.DeployTask, error) {
			return &db.DeployTask{
				ID:           "test-uuid-123",
				ProjectOwner: req.ProjectOwner,
				ProjectName:  req.ProjectName,
				Branch:       req.Branch,
				Environment:  req.Environment,
				Status:       "pending",
				CreatedAt:    now,
				UpdatedAt:    now,
			}, nil
		},
	}
	r := setupRouterWithTaskMgr(&mockGiteaClient{}, testConfig(), taskMgr)

	body := `{"project_owner":"org","project_name":"my-app","branch":"main","environment":"dev"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/deploy", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp["code"])

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "test-uuid-123", data["task_id"])
	assert.Equal(t, "pending", data["status"])
	assert.NotEmpty(t, data["created_at"])
}

func TestHandleDeploy_InvalidJSON(t *testing.T) {
	taskMgr := &mockTaskManager{}
	r := setupRouterWithTaskMgr(&mockGiteaClient{}, testConfig(), taskMgr)

	body := `{invalid json}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/deploy", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(-1), resp["code"])
	assert.Contains(t, resp["message"], "请求参数错误")
}

func TestHandleDeploy_ValidationError(t *testing.T) {
	taskMgr := &mockTaskManager{
		createTaskFn: func(req task.DeployRequest) (*db.DeployTask, error) {
			return nil, fmt.Errorf("project_owner must not be empty")
		},
	}
	r := setupRouterWithTaskMgr(&mockGiteaClient{}, testConfig(), taskMgr)

	body := `{"project_owner":"","project_name":"my-app","branch":"main","environment":"dev"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/deploy", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(-1), resp["code"])
	assert.Contains(t, resp["message"], "project_owner must not be empty")
}

func TestHandleDeployStatus_Success(t *testing.T) {
	now := time.Now().UTC()
	taskMgr := &mockTaskManager{
		getStatusFn: func(taskID string) (*task.TaskStatus, error) {
			return &task.TaskStatus{
				TaskID:      taskID,
				Status:      "building",
				ProjectName: "my-app",
				Branch:      "main",
				Environment: "dev",
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
	}
	r := setupRouterWithTaskMgr(&mockGiteaClient{}, testConfig(), taskMgr)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/deploy/test-uuid-123/status", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp["code"])

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "test-uuid-123", data["task_id"])
	assert.Equal(t, "building", data["status"])
	assert.Equal(t, "my-app", data["project_name"])
	assert.Equal(t, "main", data["branch"])
	assert.Equal(t, "dev", data["environment"])
}

func TestHandleDeployStatus_NotFound(t *testing.T) {
	taskMgr := &mockTaskManager{
		getStatusFn: func(taskID string) (*task.TaskStatus, error) {
			return nil, fmt.Errorf("task not found: %s", taskID)
		},
	}
	r := setupRouterWithTaskMgr(&mockGiteaClient{}, testConfig(), taskMgr)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/deploy/nonexistent/status", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(-1), resp["code"])
	assert.Contains(t, resp["message"], "task not found")
}

func TestHandleDeployLogs_Success(t *testing.T) {
	taskMgr := &mockTaskManager{
		getLogsFn: func(taskID string) (string, error) {
			return "[2024-01-01T00:00:00Z] [cloning] 开始拉取代码...", nil
		},
	}
	r := setupRouterWithTaskMgr(&mockGiteaClient{}, testConfig(), taskMgr)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/deploy/test-uuid-123/logs", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp["code"])

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "test-uuid-123", data["task_id"])
	assert.Contains(t, data["logs"], "开始拉取代码")
}

func TestHandleDeployLogs_NotFound(t *testing.T) {
	taskMgr := &mockTaskManager{
		getLogsFn: func(taskID string) (string, error) {
			return "", fmt.Errorf("task not found: %s", taskID)
		},
	}
	r := setupRouterWithTaskMgr(&mockGiteaClient{}, testConfig(), taskMgr)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/deploy/nonexistent/logs", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(-1), resp["code"])
	assert.Contains(t, resp["message"], "task not found")
}

func TestHandleDeployRecords_Success(t *testing.T) {
	now := time.Now().UTC()
	taskMgr := &mockTaskManager{
		listRecordsFn: func(filter task.RecordFilter) ([]task.DeployRecord, int, error) {
			records := []task.DeployRecord{
				{
					ID:           "uuid-1",
					ProjectOwner: "org",
					ProjectName:  "my-app",
					Branch:       "main",
					Environment:  "dev",
					Status:       "success",
					CreatedAt:    now,
				},
			}
			return records, 1, nil
		},
	}
	r := setupRouterWithTaskMgr(&mockGiteaClient{}, testConfig(), taskMgr)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/deploy/records", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp["code"])

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, float64(1), data["total"])

	records := data["records"].([]interface{})
	assert.Len(t, records, 1)

	first := records[0].(map[string]interface{})
	assert.Equal(t, "uuid-1", first["id"])
	assert.Equal(t, "my-app", first["project_name"])
	assert.Equal(t, "success", first["status"])
}

func TestHandleDeployRecords_WithFilters(t *testing.T) {
	var capturedFilter task.RecordFilter
	taskMgr := &mockTaskManager{
		listRecordsFn: func(filter task.RecordFilter) ([]task.DeployRecord, int, error) {
			capturedFilter = filter
			return []task.DeployRecord{}, 0, nil
		},
	}
	r := setupRouterWithTaskMgr(&mockGiteaClient{}, testConfig(), taskMgr)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/deploy/records?project=my-app&environment=dev&page=2&page_size=10", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "my-app", capturedFilter.Project)
	assert.Equal(t, "dev", capturedFilter.Environment)
	assert.Equal(t, 2, capturedFilter.Page)
	assert.Equal(t, 10, capturedFilter.PageSize)
}

func TestHandleDeployRecords_EmptyList(t *testing.T) {
	taskMgr := &mockTaskManager{
		listRecordsFn: func(filter task.RecordFilter) ([]task.DeployRecord, int, error) {
			return []task.DeployRecord{}, 0, nil
		},
	}
	r := setupRouterWithTaskMgr(&mockGiteaClient{}, testConfig(), taskMgr)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/api/deploy/records", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp["code"])

	data := resp["data"].(map[string]interface{})
	assert.Equal(t, float64(0), data["total"])

	records := data["records"].([]interface{})
	assert.Len(t, records, 0)
}
