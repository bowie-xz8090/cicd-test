package task

import (
	"database/sql"
	"fmt"
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
	database *sql.DB
	builder  builder.Builder
	deployer deployer.Deployer
	cfg      *config.AppConfig
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
	if m.builder != nil && m.deployer != nil && m.cfg != nil {
		go m.executeDeployment(task)
	}

	return task, nil
}

// executeDeployment runs the full async deployment pipeline:
// pending → cloning → building → deploying → success
// Any stage failure sets the task to "failed" with the error recorded.
func (m *taskManager) executeDeployment(task *db.DeployTask) {
	// --- Stage 1: Cloning ---
	db.UpdateTaskStatus(m.database, task.ID, "cloning")
	m.appendLog(task.ID, "cloning", "开始拉取代码...")

	// Get project config (use defaults if not found).
	projectConfig, err := m.cfg.GetProjectConfig(task.ProjectName)
	if err != nil {
		// Use default project config if not configured.
		projectConfig = config.ProjectConfig{
			BuildCmd:     "make build",
			BuildOutput:  "./dist",
			DeployScript: "",
		}
	}

	// Build repo URL from Gitea config.
	giteaCfg := m.cfg.GetGiteaConfig()
	repoURL := giteaCfg.URL + "/" + task.ProjectOwner + "/" + task.ProjectName + ".git"

	// Build work directory.
	workDir := m.cfg.Server.Workspace + "/" + task.ProjectOwner + "/" + task.ProjectName

	// Clone or pull the code.
	if err := m.builder.CloneOrPull(repoURL, task.Branch, workDir); err != nil {
		m.failTask(task.ID, fmt.Sprintf("代码拉取失败: %v", err))
		return
	}
	m.appendLog(task.ID, "cloning", "代码拉取完成")

	// --- Stage 2: Building ---
	db.UpdateTaskStatus(m.database, task.ID, "building")
	m.appendLog(task.ID, "building", "开始构建...")

	if _, err := m.builder.Build(workDir, projectConfig.BuildCmd, task.Environment); err != nil {
		m.failTask(task.ID, fmt.Sprintf("构建失败: %v", err))
		return
	}
	m.appendLog(task.ID, "building", "构建完成")

	// --- Stage 3: Deploying ---
	db.UpdateTaskStatus(m.database, task.ID, "deploying")
	m.appendLog(task.ID, "deploying", "开始部署...")

	// Get server config for the target environment.
	serverConfig, err := m.cfg.GetServerConfig(task.Environment)
	if err != nil {
		m.failTask(task.ID, fmt.Sprintf("获取服务器配置失败: %v", err))
		return
	}

	// Build artifact path.
	artifactPath := workDir + "/" + projectConfig.BuildOutput

	// Upload artifact to target server.
	if err := m.deployer.Upload(artifactPath, serverConfig); err != nil {
		m.failTask(task.ID, fmt.Sprintf("产物上传失败: %v", err))
		return
	}

	// Execute deploy script on target server.
	if _, err := m.deployer.Execute(serverConfig, projectConfig.DeployScript); err != nil {
		m.failTask(task.ID, fmt.Sprintf("部署脚本执行失败: %v", err))
		return
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
