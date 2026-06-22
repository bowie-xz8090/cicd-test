package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DeployTask represents a deployment task record in the database.
type DeployTask struct {
	ID           string     `json:"id" db:"id"`
	ProjectOwner string     `json:"project_owner" db:"project_owner"`
	ProjectName  string     `json:"project_name" db:"project_name"`
	SubProject   string     `json:"sub_project" db:"sub_project"`
	Branch       string     `json:"branch" db:"branch"`
	Environment  string     `json:"environment" db:"environment"`
	Status       string     `json:"status" db:"status"`
	Logs         string     `json:"logs" db:"logs"`
	ErrorMessage string     `json:"error_message" db:"error_message"`
	CreatedAt    time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at" db:"updated_at"`
	FinishedAt   *time.Time `json:"finished_at" db:"finished_at"`
}

// RecordFilter holds filtering and pagination parameters for listing deploy records.
type RecordFilter struct {
	Project     string `form:"project"`
	Environment string `form:"environment"`
	Page        int    `form:"page,default=1"`
	PageSize    int    `form:"page_size,default=20"`
}

const createTableSQL = `
CREATE TABLE IF NOT EXISTS deploy_tasks (
    id TEXT PRIMARY KEY,
    project_owner TEXT NOT NULL,
    project_name TEXT NOT NULL,
    sub_project TEXT DEFAULT '',
    branch TEXT NOT NULL,
    environment TEXT NOT NULL CHECK(environment IN ('dev', 'sit', 'prod')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'cloning', 'building', 'deploying', 'success', 'failed')),
    logs TEXT DEFAULT '',
    error_message TEXT DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at DATETIME
);
`

const createIndexesSQL = `
CREATE INDEX IF NOT EXISTS idx_deploy_tasks_project ON deploy_tasks(project_name);
CREATE INDEX IF NOT EXISTS idx_deploy_tasks_sub_project ON deploy_tasks(sub_project);
CREATE INDEX IF NOT EXISTS idx_deploy_tasks_environment ON deploy_tasks(environment);
CREATE INDEX IF NOT EXISTS idx_deploy_tasks_status ON deploy_tasks(status);
CREATE INDEX IF NOT EXISTS idx_deploy_tasks_created_at ON deploy_tasks(created_at);
`

const migrateAddSubProjectSQL = `
ALTER TABLE deploy_tasks ADD COLUMN sub_project TEXT DEFAULT '';
`

// InitDB opens a SQLite database at the given path and creates the deploy_tasks
// table and indexes if they don't already exist.
func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create deploy_tasks table: %w", err)
	}

	// Migration: add sub_project column if it doesn't exist (for existing databases)
	_, _ = db.Exec(migrateAddSubProjectSQL) // Ignore error if column already exists

	if _, err := db.Exec(createIndexesSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create indexes: %w", err)
	}

	return db, nil
}

// CreateTask inserts a new deploy task into the database.
// The task's CreatedAt and UpdatedAt fields are stored as RFC3339 strings.
func CreateTask(db *sql.DB, task *DeployTask) error {
	query := `
		INSERT INTO deploy_tasks (id, project_owner, project_name, sub_project, branch, environment, status, logs, error_message, created_at, updated_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	var finishedAt *string
	if task.FinishedAt != nil {
		s := task.FinishedAt.UTC().Format(time.RFC3339)
		finishedAt = &s
	}

	_, err := db.Exec(query,
		task.ID,
		task.ProjectOwner,
		task.ProjectName,
		task.SubProject,
		task.Branch,
		task.Environment,
		task.Status,
		task.Logs,
		task.ErrorMessage,
		task.CreatedAt.UTC().Format(time.RFC3339),
		task.UpdatedAt.UTC().Format(time.RFC3339),
		finishedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}
	return nil
}

// GetTaskByID retrieves a deploy task by its ID.
// Returns nil and an error if the task is not found.
func GetTaskByID(db *sql.DB, id string) (*DeployTask, error) {
	query := `
		SELECT id, project_owner, project_name, sub_project, branch, environment, status, logs, error_message, created_at, updated_at, finished_at
		FROM deploy_tasks WHERE id = ?
	`
	row := db.QueryRow(query, id)

	var task DeployTask
	var createdAt, updatedAt string
	var finishedAt *string

	err := row.Scan(
		&task.ID,
		&task.ProjectOwner,
		&task.ProjectName,
		&task.SubProject,
		&task.Branch,
		&task.Environment,
		&task.Status,
		&task.Logs,
		&task.ErrorMessage,
		&createdAt,
		&updatedAt,
		&finishedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found: %s", id)
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	task.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse created_at: %w", err)
	}
	task.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse updated_at: %w", err)
	}
	if finishedAt != nil {
		t, err := time.Parse(time.RFC3339, *finishedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to parse finished_at: %w", err)
		}
		task.FinishedAt = &t
	}

	return &task, nil
}

// UpdateTaskStatus updates the status of a deploy task.
// It also updates the updated_at timestamp. When the status is "success" or
// "failed", it additionally sets the finished_at timestamp.
func UpdateTaskStatus(db *sql.DB, id string, status string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	var query string
	var args []interface{}

	if status == "success" || status == "failed" {
		query = `UPDATE deploy_tasks SET status = ?, updated_at = ?, finished_at = ? WHERE id = ?`
		args = []interface{}{status, now, now, id}
	} else {
		query = `UPDATE deploy_tasks SET status = ?, updated_at = ? WHERE id = ?`
		args = []interface{}{status, now, id}
	}

	result, err := db.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}

// UpdateTaskLogs updates the logs of a deploy task.
// It also updates the updated_at timestamp.
func UpdateTaskLogs(db *sql.DB, id string, logs string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	query := `UPDATE deploy_tasks SET logs = ?, updated_at = ? WHERE id = ?`
	result, err := db.Exec(query, logs, now, id)
	if err != nil {
		return fmt.Errorf("failed to update task logs: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}

// UpdateTaskErrorMessage updates the error_message field of a deploy task.
// It also updates the updated_at timestamp.
func UpdateTaskErrorMessage(db *sql.DB, id string, errorMessage string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	query := `UPDATE deploy_tasks SET error_message = ?, updated_at = ? WHERE id = ?`
	result, err := db.Exec(query, errorMessage, now, id)
	if err != nil {
		return fmt.Errorf("failed to update task error message: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("task not found: %s", id)
	}

	return nil
}

// ListRecords returns a paginated list of deploy tasks matching the given filter,
// along with the total count of matching records.
func ListRecords(db *sql.DB, filter RecordFilter) ([]DeployTask, int, error) {
	// Apply defaults for pagination.
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}

	// Build WHERE clause dynamically.
	where := "WHERE 1=1"
	args := []interface{}{}

	if filter.Project != "" {
		where += " AND project_name = ?"
		args = append(args, filter.Project)
	}
	if filter.Environment != "" {
		where += " AND environment = ?"
		args = append(args, filter.Environment)
	}

	// Get total count.
	countQuery := "SELECT COUNT(*) FROM deploy_tasks " + where
	var total int
	if err := db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count records: %w", err)
	}

	// Get paginated records.
	offset := (filter.Page - 1) * filter.PageSize
	dataQuery := "SELECT id, project_owner, project_name, sub_project, branch, environment, status, logs, error_message, created_at, updated_at, finished_at FROM deploy_tasks " +
		where + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	dataArgs := append(args, filter.PageSize, offset)

	rows, err := db.Query(dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query records: %w", err)
	}
	defer rows.Close()

	var tasks []DeployTask
	for rows.Next() {
		var task DeployTask
		var createdAt, updatedAt string
		var finishedAt *string

		if err := rows.Scan(
			&task.ID,
			&task.ProjectOwner,
			&task.ProjectName,
			&task.SubProject,
			&task.Branch,
			&task.Environment,
			&task.Status,
			&task.Logs,
			&task.ErrorMessage,
			&createdAt,
			&updatedAt,
			&finishedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan record: %w", err)
		}

		task.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to parse created_at: %w", err)
		}
		task.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to parse updated_at: %w", err)
		}
		if finishedAt != nil {
			t, err := time.Parse(time.RFC3339, *finishedAt)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to parse finished_at: %w", err)
			}
			task.FinishedAt = &t
		}

		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating records: %w", err)
	}

	return tasks, total, nil
}

// ClearDeployHistory removes deployment records and compacts the SQLite database
// so the space previously occupied by logs is returned to disk.
func ClearDeployHistory(db *sql.DB) (int, error) {
	result, err := db.Exec(`DELETE FROM deploy_tasks`)
	if err != nil {
		return 0, fmt.Errorf("failed to delete deploy history: %w", err)
	}

	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get deleted record count: %w", err)
	}

	// Truncate the WAL file first, then compact the main database file.
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return 0, fmt.Errorf("failed to checkpoint deploy database: %w", err)
	}
	if _, err := db.Exec(`VACUUM`); err != nil {
		return 0, fmt.Errorf("failed to compact deploy database: %w", err)
	}

	return int(deleted), nil
}

// HasActiveTasks reports whether deployment records are still being processed.
func HasActiveTasks(db *sql.DB) (bool, error) {
	var count int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM deploy_tasks
		WHERE status IN ('pending', 'cloning', 'building', 'deploying')
	`).Scan(&count); err != nil {
		return false, fmt.Errorf("failed to check active deployment tasks: %w", err)
	}
	return count > 0, nil
}
