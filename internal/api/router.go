package api

import (
	"net/http"
	"strings"

	"auto-deploy-platform/internal/config"
	"auto-deploy-platform/internal/gitea"
	"auto-deploy-platform/internal/task"

	"github.com/gin-gonic/gin"
)

// Handler holds the dependencies for all API endpoint handlers.
type Handler struct {
	giteaClient gitea.GiteaClient
	cfg         *config.AppConfig
	taskManager task.TaskManager
}

// NewHandler creates a new Handler with the given dependencies.
func NewHandler(giteaClient gitea.GiteaClient, cfg *config.AppConfig, taskMgr task.TaskManager) *Handler {
	return &Handler{
		giteaClient: giteaClient,
		cfg:         cfg,
		taskManager: taskMgr,
	}
}

// RegisterRoutes registers all API routes on the given Gin engine.
func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.GET("/health", h.handleHealth)
		api.GET("/projects", h.handleListProjects)
		api.GET("/projects/:owner/:repo/branches", h.handleListBranches)
		api.GET("/environments", h.handleListEnvironments)

		// Deploy routes: register /deploy/records BEFORE /:id routes to avoid conflicts.
		api.GET("/deploy/records", h.handleDeployRecords)
		api.POST("/deploy", h.handleDeploy)
		api.GET("/deploy/:id/status", h.handleDeployStatus)
		api.GET("/deploy/:id/logs", h.handleDeployLogs)
	}
}

// handleHealth returns a simple health check response.
func (h *Handler) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}

// handleListProjects fetches all repositories from Gitea and returns them.
func (h *Handler) handleListProjects(c *gin.Context) {
	repos, err := h.giteaClient.ListRepos()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"code":    -1,
			"message": "获取项目列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": repos,
	})
}

// handleListBranches fetches all branches for the specified repository from Gitea.
func (h *Handler) handleListBranches(c *gin.Context) {
	owner := c.Param("owner")
	repo := c.Param("repo")

	branches, err := h.giteaClient.ListBranches(owner, repo)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"code":    -1,
			"message": "获取分支列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": branches,
	})
}

// EnvironmentItem represents a single environment entry in the API response.
type EnvironmentItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// handleListEnvironments returns the list of available deployment environments from config.
func (h *Handler) handleListEnvironments(c *gin.Context) {
	envs := make([]EnvironmentItem, 0, len(h.cfg.Environments))
	for key, envCfg := range h.cfg.Environments {
		envs = append(envs, EnvironmentItem{
			Key:   key,
			Label: envCfg.Label,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": envs,
	})
}

// handleDeploy creates a new deployment task from the request body.
func (h *Handler) handleDeploy(c *gin.Context) {
	var req task.DeployRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    -1,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	deployTask, err := h.taskManager.CreateTask(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    -1,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"task_id":    deployTask.ID,
			"status":     deployTask.Status,
			"created_at": deployTask.CreatedAt,
		},
	})
}

// handleDeployStatus returns the current status of a deployment task.
func (h *Handler) handleDeployStatus(c *gin.Context) {
	id := c.Param("id")

	status, err := h.taskManager.GetTaskStatus(id)
	if err != nil {
		if strings.Contains(err.Error(), "task not found") {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    -1,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    -1,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": status,
	})
}

// handleDeployLogs returns the execution logs of a deployment task.
func (h *Handler) handleDeployLogs(c *gin.Context) {
	id := c.Param("id")

	logs, err := h.taskManager.GetTaskLogs(id)
	if err != nil {
		if strings.Contains(err.Error(), "task not found") {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    -1,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    -1,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"task_id": id,
			"logs":    logs,
		},
	})
}

// handleDeployRecords returns a paginated list of deployment records with optional filtering.
func (h *Handler) handleDeployRecords(c *gin.Context) {
	var filter task.RecordFilter
	if err := c.ShouldBindQuery(&filter); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    -1,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	records, total, err := h.taskManager.ListRecords(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    -1,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"total":   total,
			"records": records,
		},
	})
}
