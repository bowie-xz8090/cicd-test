package task

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"auto-deploy-platform/internal/builder"
	"auto-deploy-platform/internal/config"
	"auto-deploy-platform/internal/db"
	"auto-deploy-platform/internal/deployer"

	"github.com/google/uuid"
)

// DeployRequest represents a request to create a new deployment task.
type DeployRequest struct {
	ProjectOwner string `json:"project_owner"`
	ProjectName  string `json:"project_name"`
	Branch       string `json:"branch"`
	Environment  string `json:"environment"`
}

// TaskStatus represents the current status of a deployment task.
type TaskStatus struct {
	TaskID      string    `json:"task_id"`
	Status      string    `json:"status"`
	ProjectName string    `json:"project_name"`
	Branch      string    `json:"branch"`
	Environment string    `json:"environment"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// DeployRecord represents a deployment record for list results.
type DeployRecord struct {
	ID           string     `json:"id"`
	ProjectOwner string     `json:"project_owner"`
	ProjectName  string     `json:"project_name"`
	Branch       string     `json:"branch"`
	Environment  string     `json:"environment"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}

// RecordFilter holds filtering and pagination parameters for listing deploy records.
type RecordFilter struct {
	Project     string `form:"project"`
	Environment string `form:"environment"`
	Page        int    `form:"page,default=1"`
	PageSize    int    `form:"page_size,default=20"`
}

// TaskManager defines the interface for managing deployment tasks.
type TaskManager interface {
	CreateTask(req DeployRequest) (*db.DeployTask, error)
	GetTaskStatus(taskID string) (*TaskStatus, error)
	GetTaskLogs(taskID string) (string, error)
	ListRecords(filter RecordFilter) ([]DeployRecord, int, error)
	CancelTask(taskID string) error
}

// validEnvironments defines the allowed deployment environments.
var validEnvironments = map[string]bool{
	"dev":  true,
	"sit":  true,
	"prod": true,
}

// validTransitions defines the allowed state transitions for deployment tasks.
var validTransitions = map[string]map[string]bool{
	"pending": {
		"cloning": true,
		"failed":  true,
	},
	"cloning": {
		"building": true,
		"failed":   true,
	},
	"building": {
		"deploying": true,
		"failed":    true,
	},
	"deploying": {
		"success": true,
		"failed":  true,
	},
}

// IsValidTransition checks whether a state transition from one status to another is allowed.
func IsValidTransition(from, to string) bool {
	targets, ok := validTransitions[from]
	if !ok {
		return false
	}
	return targets[to]
}

// taskManager is the concrete implementation of TaskManager.
type taskManager struct {
	database   *sql.DB
	builder    builder.Builder
	deployer   deployer.Deployer
	cfg        *config.AppConfig
	cfgManager *config.Manager
	cancelMap  sync.Map // taskID -> context.CancelFunc
}

// NewTaskManager creates a new TaskManager instance.
// The builder, deployer, and cfg parameters may be nil if async deployment is not needed
// (e.g., in tests that only test task creation and querying).
func NewTaskManager(database *sql.DB, bldr builder.Builder, dplyr deployer.Deployer, cfg *config.AppConfig) TaskManager {
	return &taskManager{
		database: database,
		builder:  bldr,
		deployer: dplyr,
		cfg:      cfg,
	}
}

// NewTaskManagerWithConfigManager creates a TaskManager that reads config dynamically from the Manager.
func NewTaskManagerWithConfigManager(database *sql.DB, bldr builder.Builder, dplyr deployer.Deployer, cfgMgr *config.Manager) TaskManager {
	return &taskManager{
		database:   database,
		builder:    bldr,
		deployer:   dplyr,
		cfg:        cfgMgr.Get(),
		cfgManager: cfgMgr,
	}
}

// ctxErrMsg returns an appropriate error message based on context error type.
func ctxErrMsg(ctx context.Context) string {
	if ctx.Err() == context.DeadlineExceeded {
		return "部署超时（超过30分钟），已自动中断"
	}
	return "任务已被取消"
}

// getConfig returns the latest config, preferring the Manager if available.
func (m *taskManager) getConfig() *config.AppConfig {
	if m.cfgManager != nil {
		return m.cfgManager.Get()
	}
	return m.cfg
}

// CreateTask validates the deploy request, creates a new task record with status "pending",
// and launches the async deployment pipeline.
func (m *taskManager) CreateTask(req DeployRequest) (*db.DeployTask, error) {
	// Validate required fields.
	if req.ProjectOwner == "" {
		return nil, fmt.Errorf("project_owner must not be empty")
	}
	if req.ProjectName == "" {
		return nil, fmt.Errorf("project_name must not be empty")
	}
	if req.Branch == "" {
		return nil, fmt.Errorf("branch must not be empty")
	}
	if req.Environment == "" {
		return nil, fmt.Errorf("environment must not be empty")
	}

	// Validate environment value.
	if !validEnvironments[req.Environment] {
		return nil, fmt.Errorf("invalid environment %q: must be one of dev, sit, prod", req.Environment)
	}

	now := time.Now().UTC()
	task := &db.DeployTask{
		ID:           uuid.New().String(),
		ProjectOwner: req.ProjectOwner,
		ProjectName:  req.ProjectName,
		Branch:       req.Branch,
		Environment:  req.Environment,
		Status:       "pending",
		Logs:         "",
		ErrorMessage: "",
		CreatedAt:    now,
		UpdatedAt:    now,
		FinishedAt:   nil,
	}

	if err := db.CreateTask(m.database, task); err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	// Launch async deployment if builder and deployer are available.
	if m.builder != nil && m.deployer != nil && m.getConfig() != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		m.cancelMap.Store(task.ID, cancel)
		go m.executeDeployment(ctx, task)
	}

	return task, nil
}

// executeDeployment runs the full async deployment pipeline:
// pending -> cloning -> building -> deploying -> success
// Any stage failure or cancellation sets the task to "failed" with the error recorded.
func (m *taskManager) executeDeployment(ctx context.Context, task *db.DeployTask) {
	defer m.cancelMap.Delete(task.ID)

	// Get the latest config at the start of deployment.
	cfg := m.getConfig()

	// --- Stage 1: Cloning ---
	db.UpdateTaskStatus(m.database, task.ID, "cloning")
	m.appendLog(task.ID, "cloning", "开始拉取代码...")

	if ctx.Err() != nil {
		m.failTask(task.ID, ctxErrMsg(ctx))
		return
	}

	// Get project config (use defaults if not found).
	projectConfig, err := cfg.GetProjectConfigForEnv(task.ProjectName, task.Environment)
	if err != nil {
		// Use default project config if not configured.
		projectConfig = config.ProjectConfig{
			BuildCmd:     "make build",
			BuildOutput:  "./dist",
			DeployScript: "",
		}
	}

	// Build repo URL from Gitea config.
	giteaCfg := cfg.GetGiteaConfig()
	repoURL := giteaCfg.URL + "/" + task.ProjectOwner + "/" + task.ProjectName + ".git"

	// Build work directory.
	workDir := cfg.Server.Workspace + "/" + task.ProjectOwner + "/" + task.ProjectName

	// Clone or pull the code.
	if err := m.builder.CloneOrPull(repoURL, task.Branch, workDir); err != nil {
		m.failTask(task.ID, fmt.Sprintf("代码拉取失败: %v", err))
		return
	}
	m.appendLog(task.ID, "cloning", "代码拉取完成")

	// --- Stage 2: Building ---
	if ctx.Err() != nil {
		m.failTask(task.ID, ctxErrMsg(ctx))
		return
	}
	db.UpdateTaskStatus(m.database, task.ID, "building")
	m.appendLog(task.ID, "building", "开始构建...")

	if _, err := m.builder.Build(workDir, projectConfig.BuildCmd, task.Environment); err != nil {
		m.failTask(task.ID, fmt.Sprintf("构建失败: %v", err))
		return
	}
	m.appendLog(task.ID, "building", "构建完成")

	// --- Stage 3: Deploying ---
	if ctx.Err() != nil {
		m.failTask(task.ID, ctxErrMsg(ctx))
		return
	}
	db.UpdateTaskStatus(m.database, task.ID, "deploying")
	m.appendLog(task.ID, "deploying", "开始部署...")

	// Get server config for the target environment.
	serverConfig, err := cfg.GetServerConfig(task.Environment)
	if err != nil {
		m.failTask(task.ID, fmt.Sprintf("获取服务器配置失败: %v", err))
		return
	}

	// Multi-artifact deploy: if artifacts list is configured, deploy each one.
	if len(projectConfig.Artifacts) > 0 {
		for i, artifact := range projectConfig.Artifacts {
			m.appendLog(task.ID, "deploying", fmt.Sprintf("部署产物 [%d/%d]: %s", i+1, len(projectConfig.Artifacts), artifact.BuildOutput))

			artifactPath := workDir + "/" + artifact.BuildOutput

			// If artifact is a directory, tar it first.
			artifactPath, err = m.prepareArtifact(artifactPath, artifact.RenameTo)
			if err != nil {
				m.failTask(task.ID, fmt.Sprintf("产物准备失败 [%s]: %v", artifact.BuildOutput, err))
				return
			}

			// Upload artifact.
			if err := m.deployer.Upload(artifactPath, serverConfig); err != nil {
				m.failTask(task.ID, fmt.Sprintf("产物上传失败 [%s]: %v", artifact.BuildOutput, err))
				return
			}

			// Execute deploy script.
			if artifact.DeployScript != "" {
				if _, err := m.deployer.Execute(serverConfig, artifact.DeployScript); err != nil {
					m.failTask(task.ID, fmt.Sprintf("部署脚本执行失败 [%s]: %v", artifact.BuildOutput, err))
					return
				}
			}
		}
	} else {
		// Single artifact deploy (original logic).
		artifactPath := workDir + "/" + projectConfig.BuildOutput

		// Prepare artifact (handles directory -> tar.gz, and rename).
		artifactPath, err = m.prepareArtifact(artifactPath, projectConfig.RenameTo)
		if err != nil {
			m.failTask(task.ID, fmt.Sprintf("产物准备失败: %v", err))
			return
		}

		// Upload artifact.
		if err := m.deployer.Upload(artifactPath, serverConfig); err != nil {
			m.failTask(task.ID, fmt.Sprintf("产物上传失败: %v", err))
			return
		}

		// Execute deploy script.
		if projectConfig.DeployScript != "" {
			if _, err := m.deployer.Execute(serverConfig, projectConfig.DeployScript); err != nil {
				m.failTask(task.ID, fmt.Sprintf("部署脚本执行失败: %v", err))
				return
			}
		}
	}

	m.appendLog(task.ID, "deploying", "部署完成")

	// --- Success ---
	db.UpdateTaskStatus(m.database, task.ID, "success")
}

// appendLog appends a formatted log line to the task's logs in the database.
// Log format: [2006-01-02T15:04:05Z] [stage] message
func (m *taskManager) appendLog(taskID, stage, message string) {
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	logLine := fmt.Sprintf("[%s] [%s] %s", timestamp, stage, message)

	// Get current logs to append.
	task, err := db.GetTaskByID(m.database, taskID)
	if err != nil {
		return
	}

	newLogs := task.Logs
	if newLogs != "" {
		newLogs += "\n"
	}
	newLogs += logLine

	db.UpdateTaskLogs(m.database, taskID, newLogs)
}

// failTask sets the task status to "failed" and records the error message.
func (m *taskManager) failTask(taskID, errorMsg string) {
	m.appendLog(taskID, "error", errorMsg)
	db.UpdateTaskStatus(m.database, taskID, "failed")
	db.UpdateTaskErrorMessage(m.database, taskID, errorMsg)
}

// GetTaskStatus retrieves the current status of a deployment task by its ID.
func (m *taskManager) GetTaskStatus(taskID string) (*TaskStatus, error) {
	task, err := db.GetTaskByID(m.database, taskID)
	if err != nil {
		return nil, err
	}

	return &TaskStatus{
		TaskID:      task.ID,
		Status:      task.Status,
		ProjectName: task.ProjectName,
		Branch:      task.Branch,
		Environment: task.Environment,
		CreatedAt:   task.CreatedAt,
		UpdatedAt:   task.UpdatedAt,
	}, nil
}

// CancelTask cancels a running deployment task.
// Only tasks in cloning/building/deploying status can be cancelled.
// Regardless of whether the running context is found, the task will be marked as failed.
func (m *taskManager) CancelTask(taskID string) error {
	task, err := db.GetTaskByID(m.database, taskID)
	if err != nil {
		return fmt.Errorf("任务不存在: %w", err)
	}

	// Only allow cancelling tasks that are in progress
	if task.Status != "cloning" && task.Status != "building" && task.Status != "deploying" {
		return fmt.Errorf("任务当前状态为 %q，无法中断（仅允许中断执行中的任务）", task.Status)
	}

	// Try to cancel the running goroutine if context exists
	if cancel, ok := m.cancelMap.Load(taskID); ok {
		cancel.(context.CancelFunc)()
	}

	// Always mark the task as failed, regardless of whether context was found
	m.failTask(taskID, "任务已被用户中断")
	return nil
}

// GetTaskLogs retrieves the execution logs of a deployment task by its ID.
func (m *taskManager) GetTaskLogs(taskID string) (string, error) {
	task, err := db.GetTaskByID(m.database, taskID)
	if err != nil {
		return "", err
	}

	return task.Logs, nil
}

// ListRecords returns a paginated list of deployment records matching the given filter,
// along with the total count of matching records.
func (m *taskManager) ListRecords(filter RecordFilter) ([]DeployRecord, int, error) {
	dbFilter := db.RecordFilter{
		Project:     filter.Project,
		Environment: filter.Environment,
		Page:        filter.Page,
		PageSize:    filter.PageSize,
	}

	tasks, total, err := db.ListRecords(m.database, dbFilter)
	if err != nil {
		return nil, 0, err
	}

	records := make([]DeployRecord, len(tasks))
	for i, t := range tasks {
		records[i] = DeployRecord{
			ID:           t.ID,
			ProjectOwner: t.ProjectOwner,
			ProjectName:  t.ProjectName,
			Branch:       t.Branch,
			Environment:  t.Environment,
			Status:       t.Status,
			CreatedAt:    t.CreatedAt,
			FinishedAt:   t.FinishedAt,
		}
	}

	return records, total, nil
}

// prepareArtifact checks if the artifact path is a directory or file.
// If it's a directory, it creates a tar.gz archive and returns the archive path.
// If renameTo is set and the artifact is a file, it renames the file.
// Returns the final path to upload.
func (m *taskManager) prepareArtifact(artifactPath string, renameTo string) (string, error) {
	info, err := os.Stat(artifactPath)
	if err != nil {
		return "", fmt.Errorf("产物不存在: %s: %w", artifactPath, err)
	}

	if info.IsDir() {
		// Directory: create a tar.gz archive
		tarPath := artifactPath + ".tar.gz"
		if err := createTarGz(tarPath, artifactPath); err != nil {
			return "", fmt.Errorf("打包目录失败: %w", err)
		}
		// If renameTo is set, rename the tar.gz
		if renameTo != "" {
			dir := filepath.Dir(tarPath)
			newPath := filepath.Join(dir, renameTo)
			if err := os.Rename(tarPath, newPath); err != nil {
				return "", fmt.Errorf("重命名失败: %w", err)
			}
			return newPath, nil
		}
		return tarPath, nil
	}

	// Single file: rename if needed
	if renameTo != "" {
		dir := filepath.Dir(artifactPath)
		newPath := filepath.Join(dir, renameTo)
		if err := os.Rename(artifactPath, newPath); err != nil {
			return "", fmt.Errorf("重命名失败: %w", err)
		}
		return newPath, nil
	}

	return artifactPath, nil
}

// createTarGz creates a tar.gz archive of the given source directory.
func createTarGz(tarPath string, sourceDir string) error {
	tarFile, err := os.Create(tarPath)
	if err != nil {
		return err
	}
	defer tarFile.Close()

	gzWriter := gzip.NewWriter(tarFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}

		// Use relative path inside the archive
		relPath, err := filepath.Rel(filepath.Dir(sourceDir), path)
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		// Write file content (skip directories)
		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)
		return err
	})
}
