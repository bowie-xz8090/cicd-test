package task

import (
	"os"
	"strings"
	"testing"

	"auto-deploy-platform/internal/config"
	"auto-deploy-platform/internal/db"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestDB creates a temporary SQLite database for testing and returns
// the TaskManager and a cleanup function.
func setupTestDB(t *testing.T) (TaskManager, func()) {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "task_test_*.db")
	require.NoError(t, err)
	tmpFile.Close()

	database, err := db.InitDB(tmpFile.Name())
	require.NoError(t, err)

	mgr := NewTaskManager(database, nil, nil, nil)

	cleanup := func() {
		database.Close()
		os.Remove(tmpFile.Name())
	}

	return mgr, cleanup
}

func validRequest() DeployRequest {
	return DeployRequest{
		ProjectOwner: "test-org",
		ProjectName:  "test-project",
		Branch:       "main",
		Environment:  "dev",
	}
}

func TestBuildRepoURLUsesGiteaCredentials(t *testing.T) {
	repoURL := buildRepoURL(config.GiteaConfig{
		URL:      "http://gitea.example.com",
		Username: "deploy-user",
		Token:    "token with spaces",
	}, "admin", "cde-monorepo")

	assert.True(t, strings.HasPrefix(repoURL, "http://deploy-user:token%20with%20spaces@gitea.example.com/"))
	assert.True(t, strings.HasSuffix(repoURL, "/admin/cde-monorepo.git"))
}

func TestBuildRepoURLFallsBackToOwnerAsUsername(t *testing.T) {
	repoURL := buildRepoURL(config.GiteaConfig{
		URL:   "http://gitea.example.com/",
		Token: "token",
	}, "admin", "cde-monorepo")

	assert.Equal(t, "http://admin:token@gitea.example.com/admin/cde-monorepo.git", repoURL)
}

func TestCreateTask_Success(t *testing.T) {
	mgr, cleanup := setupTestDB(t)
	defer cleanup()

	req := validRequest()
	task, err := mgr.CreateTask(req)

	require.NoError(t, err)
	assert.NotEmpty(t, task.ID)
	assert.Equal(t, req.ProjectOwner, task.ProjectOwner)
	assert.Equal(t, req.ProjectName, task.ProjectName)
	assert.Equal(t, req.Branch, task.Branch)
	assert.Equal(t, req.Environment, task.Environment)
	assert.Equal(t, "pending", task.Status)
	assert.Empty(t, task.Logs)
	assert.Empty(t, task.ErrorMessage)
	assert.Nil(t, task.FinishedAt)
}

func TestCreateTask_ValidationErrors(t *testing.T) {
	mgr, cleanup := setupTestDB(t)
	defer cleanup()

	tests := []struct {
		name   string
		req    DeployRequest
		errMsg string
	}{
		{
			name:   "empty project_owner",
			req:    DeployRequest{ProjectOwner: "", ProjectName: "p", Branch: "b", Environment: "dev"},
			errMsg: "project_owner must not be empty",
		},
		{
			name:   "empty project_name",
			req:    DeployRequest{ProjectOwner: "o", ProjectName: "", Branch: "b", Environment: "dev"},
			errMsg: "project_name must not be empty",
		},
		{
			name:   "empty branch",
			req:    DeployRequest{ProjectOwner: "o", ProjectName: "p", Branch: "", Environment: "dev"},
			errMsg: "branch must not be empty",
		},
		{
			name:   "empty environment",
			req:    DeployRequest{ProjectOwner: "o", ProjectName: "p", Branch: "b", Environment: ""},
			errMsg: "environment must not be empty",
		},
		{
			name:   "invalid environment",
			req:    DeployRequest{ProjectOwner: "o", ProjectName: "p", Branch: "b", Environment: "staging"},
			errMsg: "invalid environment",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task, err := mgr.CreateTask(tc.req)
			assert.Nil(t, task)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.errMsg)
		})
	}
}

func TestCreateTask_AllEnvironments(t *testing.T) {
	mgr, cleanup := setupTestDB(t)
	defer cleanup()

	for _, env := range []string{"dev", "sit", "prod"} {
		t.Run(env, func(t *testing.T) {
			req := validRequest()
			req.Environment = env
			task, err := mgr.CreateTask(req)
			require.NoError(t, err)
			assert.Equal(t, env, task.Environment)
		})
	}
}

func TestGetTaskStatus_Success(t *testing.T) {
	mgr, cleanup := setupTestDB(t)
	defer cleanup()

	req := validRequest()
	task, err := mgr.CreateTask(req)
	require.NoError(t, err)

	status, err := mgr.GetTaskStatus(task.ID)
	require.NoError(t, err)
	assert.Equal(t, task.ID, status.TaskID)
	assert.Equal(t, "pending", status.Status)
	assert.Equal(t, req.ProjectName, status.ProjectName)
	assert.Equal(t, req.Branch, status.Branch)
	assert.Equal(t, req.Environment, status.Environment)
}

func TestGetTaskStatus_NotFound(t *testing.T) {
	mgr, cleanup := setupTestDB(t)
	defer cleanup()

	status, err := mgr.GetTaskStatus("nonexistent-id")
	assert.Nil(t, status)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
}

func TestGetTaskLogs_Success(t *testing.T) {
	mgr, cleanup := setupTestDB(t)
	defer cleanup()

	req := validRequest()
	task, err := mgr.CreateTask(req)
	require.NoError(t, err)

	logs, err := mgr.GetTaskLogs(task.ID)
	require.NoError(t, err)
	assert.Empty(t, logs)
}

func TestGetTaskLogs_NotFound(t *testing.T) {
	mgr, cleanup := setupTestDB(t)
	defer cleanup()

	logs, err := mgr.GetTaskLogs("nonexistent-id")
	assert.Empty(t, logs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task not found")
}

func TestListRecords_Empty(t *testing.T) {
	mgr, cleanup := setupTestDB(t)
	defer cleanup()

	records, total, err := mgr.ListRecords(RecordFilter{})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.Empty(t, records)
}

func TestListRecords_WithRecords(t *testing.T) {
	mgr, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a few tasks.
	for i := 0; i < 3; i++ {
		req := validRequest()
		_, err := mgr.CreateTask(req)
		require.NoError(t, err)
	}

	records, total, err := mgr.ListRecords(RecordFilter{})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, records, 3)

	// Verify record fields are populated.
	for _, r := range records {
		assert.NotEmpty(t, r.ID)
		assert.Equal(t, "test-org", r.ProjectOwner)
		assert.Equal(t, "test-project", r.ProjectName)
		assert.Equal(t, "main", r.Branch)
		assert.Equal(t, "dev", r.Environment)
		assert.Equal(t, "pending", r.Status)
	}
}

func TestListRecords_FilterByProject(t *testing.T) {
	mgr, cleanup := setupTestDB(t)
	defer cleanup()

	// Create tasks for different projects.
	req1 := DeployRequest{ProjectOwner: "org", ProjectName: "alpha", Branch: "main", Environment: "dev"}
	req2 := DeployRequest{ProjectOwner: "org", ProjectName: "beta", Branch: "main", Environment: "dev"}
	_, err := mgr.CreateTask(req1)
	require.NoError(t, err)
	_, err = mgr.CreateTask(req2)
	require.NoError(t, err)

	records, total, err := mgr.ListRecords(RecordFilter{Project: "alpha"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, records, 1)
	assert.Equal(t, "alpha", records[0].ProjectName)
}

func TestListRecords_FilterByEnvironment(t *testing.T) {
	mgr, cleanup := setupTestDB(t)
	defer cleanup()

	req1 := DeployRequest{ProjectOwner: "org", ProjectName: "proj", Branch: "main", Environment: "dev"}
	req2 := DeployRequest{ProjectOwner: "org", ProjectName: "proj", Branch: "main", Environment: "prod"}
	_, err := mgr.CreateTask(req1)
	require.NoError(t, err)
	_, err = mgr.CreateTask(req2)
	require.NoError(t, err)

	records, total, err := mgr.ListRecords(RecordFilter{Environment: "prod"})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, records, 1)
	assert.Equal(t, "prod", records[0].Environment)
}

// Tests for IsValidTransition

func TestIsValidTransition_ValidPaths(t *testing.T) {
	validCases := []struct {
		from string
		to   string
	}{
		{"pending", "cloning"},
		{"cloning", "building"},
		{"building", "deploying"},
		{"deploying", "success"},
		{"pending", "failed"},
		{"cloning", "failed"},
		{"building", "failed"},
		{"deploying", "failed"},
	}

	for _, tc := range validCases {
		t.Run(tc.from+"→"+tc.to, func(t *testing.T) {
			assert.True(t, IsValidTransition(tc.from, tc.to))
		})
	}
}

func TestIsValidTransition_InvalidPaths(t *testing.T) {
	invalidCases := []struct {
		from string
		to   string
	}{
		{"pending", "building"},
		{"pending", "deploying"},
		{"pending", "success"},
		{"cloning", "pending"},
		{"cloning", "deploying"},
		{"cloning", "success"},
		{"building", "pending"},
		{"building", "cloning"},
		{"building", "success"},
		{"deploying", "pending"},
		{"deploying", "cloning"},
		{"deploying", "building"},
		{"success", "pending"},
		{"success", "failed"},
		{"failed", "pending"},
		{"failed", "cloning"},
		{"", "pending"},
		{"unknown", "failed"},
	}

	for _, tc := range invalidCases {
		t.Run(tc.from+"→"+tc.to, func(t *testing.T) {
			assert.False(t, IsValidTransition(tc.from, tc.to))
		})
	}
}
