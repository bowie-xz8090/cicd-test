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
	cfgManager  *config.Manager
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

// NewHandlerWithManager creates a new Handler with a config Manager for dynamic config support.
func NewHandlerWithManager(giteaClient gitea.GiteaClient, cfgMgr *config.Manager, taskMgr task.TaskManager) *Handler {
	return &Handler{
		giteaClient: giteaClient,
		cfg:         cfgMgr.Get(),
		cfgManager:  cfgMgr,
		taskManager: taskMgr,
	}
}

// getLatestConfig returns the most up-to-date config, preferring the Manager if available.
func (h *Handler) getLatestConfig() *config.AppConfig {
	if h.cfgManager != nil {
		return h.cfgManager.Get()
	}
	return h.cfg
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
		api.POST("/deploy/:id/cancel", h.handleDeployCancel)
		api.GET("/deploy/:id/status", h.handleDeployStatus)
		api.GET("/deploy/:id/logs", h.handleDeployLogs)

		// Config management routes (require admin_token authentication)
		configGroup := api.Group("/config")
		configGroup.Use(h.adminAuthMiddleware())
		{
			configGroup.PUT("", h.handleUpdateConfig)
			configGroup.POST("/reload", h.handleReloadConfig)
		}
	}
}

// handleHealth returns a simple health check response.
func (h *Handler) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": "ok"})
}

// handleListProjects fetches all repositories from Gitea and filters to only those
// configured in config.yaml.
func (h *Handler) handleListProjects(c *gin.Context) {
	repos, err := h.giteaClient.ListRepos()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"code":    -1,
			"message": "获取项目列表失败: " + err.Error(),
		})
		return
	}

	// Filter to only repos configured in config.yaml
	cfg := h.getLatestConfig()
	configuredProjects := cfg.Projects
	filtered := make([]gitea.Repository, 0, len(configuredProjects))
	for _, repo := range repos {
		if _, ok := configuredProjects[repo.Name]; ok {
			filtered = append(filtered, repo)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": filtered,
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
	Key      string `json:"key"`
	Label    string `json:"label"`
	Disabled bool   `json:"disabled"`
	UserURL  string `json:"user_url"`
	AdminURL string `json:"admin_url"`
}

// handleListEnvironments returns the list of available deployment environments from config.
// The order is fixed: dev, sit, prod.
func (h *Handler) handleListEnvironments(c *gin.Context) {
	cfg := h.getLatestConfig()

	// Fixed order
	order := []string{"dev", "sit", "prod"}
	envs := make([]EnvironmentItem, 0, len(cfg.Environments))

	for _, key := range order {
		if envCfg, ok := cfg.Environments[key]; ok {
			envs = append(envs, EnvironmentItem{
				Key:      key,
				Label:    envCfg.Label,
				Disabled: envCfg.Disabled,
				UserURL:  envCfg.Links.UserURL,
				AdminURL: envCfg.Links.AdminURL,
			})
		}
	}

	// Append any extra environments not in the fixed order
	for key, envCfg := range cfg.Environments {
		if key != "dev" && key != "sit" && key != "prod" {
			envs = append(envs, EnvironmentItem{
				Key:      key,
				Label:    envCfg.Label,
				Disabled: envCfg.Disabled,
				UserURL:  envCfg.Links.UserURL,
				AdminURL: envCfg.Links.AdminURL,
			})
		}
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

// handleDeployCancel attempts to cancel a running deployment task.
func (h *Handler) handleDeployCancel(c *gin.Context) {
	id := c.Param("id")

	if err := h.taskManager.CancelTask(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    -1,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "已发送取消信号，任务将在当前阶段完成后终止",
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

// adminAuthMiddleware returns a Gin middleware that validates the admin token.
// The token can be provided via:
//   - Header: Authorization: Bearer <token>
//   - Query parameter: ?token=<token>
func (h *Handler) adminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get the admin token from config
		var adminToken string
		if h.cfgManager != nil {
			adminToken = h.cfgManager.Get().AdminToken
		} else if h.cfg != nil {
			adminToken = h.cfg.AdminToken
		}

		// If no admin_token is configured, reject all requests
		if adminToken == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    -1,
				"message": "配置管理接口未启用：请在 config.yaml 中设置 admin_token",
			})
			return
		}

		// Extract token from request
		token := ""
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
		if token == "" {
			token = c.Query("token")
		}

		// Validate
		if token == "" || token != adminToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    -1,
				"message": "未授权：admin_token 无效",
			})
			return
		}

		c.Next()
	}
}

// handleUpdateConfig updates the configuration with the provided JSON body and persists to disk.
func (h *Handler) handleUpdateConfig(c *gin.Context) {
	if h.cfgManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    -1,
			"message": "配置管理器未初始化，不支持动态修改",
		})
		return
	}

	var newCfg config.AppConfig
	if err := c.ShouldBindJSON(&newCfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    -1,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	if err := h.cfgManager.Update(&newCfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    -1,
			"message": "配置更新失败: " + err.Error(),
		})
		return
	}

	// Update the local reference
	h.cfg = h.cfgManager.Get()

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "配置已更新并保存",
		"data":    h.cfgManager.Get(),
	})
}

// handleReloadConfig reloads the configuration from disk without restarting the service.
func (h *Handler) handleReloadConfig(c *gin.Context) {
	if h.cfgManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    -1,
			"message": "配置管理器未初始化，不支持动态重载",
		})
		return
	}

	cfg, err := h.cfgManager.Reload()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    -1,
			"message": "配置重载失败: " + err.Error(),
		})
		return
	}

	// Update the local reference
	h.cfg = cfg

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "配置已从磁盘重新加载",
		"data":    cfg,
	})
}
