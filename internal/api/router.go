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
		api.GET("/site-info", h.handleSiteInfo)
		api.GET("/projects", h.handleListProjects)
		api.GET("/projects/:owner/:repo/branches", h.handleListBranches)
		api.GET("/projects/:owner/:repo/tags", h.handleListTags)
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

// handleSiteInfo returns site configuration (title, etc.) for the frontend.
func (h *Handler) handleSiteInfo(c *gin.Context) {
	cfg := h.getLatestConfig()
	title := cfg.Server.Title
	if title == "" {
		title = "自动部署平台"
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"title": title,
		},
	})
}

// ProjectItem represents a project entry in the flattened API response.
// Each item is a deployable unit: "project_label - sub_project_label".
type ProjectItem struct {
	Owner      string `json:"owner"`
	Name       string `json:"name"`
	FullName   string `json:"full_name"`
	Label      string `json:"label"`
	SubProject string `json:"sub_project"`
	CloneURL   string `json:"clone_url"`
}

// handleListProjects fetches all repositories from Gitea and returns a flattened list
// where each entry is a "project - sub_project" deployable unit.
func (h *Handler) handleListProjects(c *gin.Context) {
	repos, err := h.giteaClient.ListRepos()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"code":    -1,
			"message": "获取项目列表失败: " + err.Error(),
		})
		return
	}

	cfg := h.getLatestConfig()
	items := make([]ProjectItem, 0)

	for _, repo := range repos {
		projCfg, ok := cfg.Projects[repo.Name]
		if !ok {
			continue
		}

		projLabel := projCfg.Label
		if projLabel == "" {
			projLabel = repo.Name
		}

		for spKey, spCfg := range projCfg.SubProjects {
			spLabel := spCfg.Label
			if spLabel == "" {
				spLabel = spKey
			}
			items = append(items, ProjectItem{
				Owner:      repo.Owner,
				Name:       repo.Name,
				FullName:   repo.FullName,
				Label:      projLabel + " - " + spLabel,
				SubProject: spKey,
				CloneURL:   repo.CloneURL,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": items,
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

// handleListTags fetches all tags for the specified repository from Gitea.
func (h *Handler) handleListTags(c *gin.Context) {
	owner := c.Param("owner")
	repo := c.Param("repo")

	tags, err := h.giteaClient.ListTags(owner, repo)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"code":    -1,
			"message": "获取标签列表失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": tags,
	})
}

// EnvironmentItem represents a single environment entry in the API response.
type EnvironmentItem struct {
	Key      string     `json:"key"`
	Label    string     `json:"label"`
	Disabled bool       `json:"disabled"`
	UserURL  string     `json:"user_url"`
	AdminURL string     `json:"admin_url"`
	Extra    []LinkItem `json:"extra"`
}

// LinkItem represents a custom link in the API response.
type LinkItem struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// toLinkItems converts config EnvLink slice to API LinkItem slice.
func toLinkItems(links []config.EnvLink) []LinkItem {
	if len(links) == 0 {
		return nil
	}
	items := make([]LinkItem, len(links))
	for i, l := range links {
		items[i] = LinkItem{Label: l.Label, URL: l.URL}
	}
	return items
}

// handleListEnvironments returns the list of available deployment environments.
// Supports optional query params:
//   - project: project name
//   - sub_project: sub-project key
//
// When both are specified, environments not configured for that sub-project are marked disabled.
func (h *Handler) handleListEnvironments(c *gin.Context) {
	cfg := h.getLatestConfig()
	projectName := c.Query("project")
	subProjectName := c.Query("sub_project")

	// Fixed order
	order := []string{"dev", "sit", "prod"}
	envs := make([]EnvironmentItem, 0, len(cfg.Environments))

	for _, key := range order {
		envCfg, ok := cfg.Environments[key]
		if !ok {
			continue
		}

		item := EnvironmentItem{
			Key:      key,
			Label:    envCfg.Label,
			Disabled: envCfg.Disabled,
			UserURL:  envCfg.Links.UserURL,
			AdminURL: envCfg.Links.AdminURL,
			Extra:    toLinkItems(envCfg.Links.Extra),
		}

		// If project+sub_project specified, check if this env is configured
		if projectName != "" && subProjectName != "" {
			if projCfg, ok := cfg.Projects[projectName]; ok {
				if subProjCfg, ok := projCfg.SubProjects[subProjectName]; ok {
					if envOverride, ok := subProjCfg.EnvOverrides[key]; ok {
						if envOverride.Disabled != nil {
							item.Disabled = *envOverride.Disabled
						}
					} else {
						item.Disabled = true
					}
				}
			}
		}

		envs = append(envs, item)
	}

	// Append any extra environments not in the fixed order
	for key, envCfg := range cfg.Environments {
		if key != "dev" && key != "sit" && key != "prod" {
			item := EnvironmentItem{
				Key:      key,
				Label:    envCfg.Label,
				Disabled: envCfg.Disabled,
				UserURL:  envCfg.Links.UserURL,
				AdminURL: envCfg.Links.AdminURL,
				Extra:    toLinkItems(envCfg.Links.Extra),
			}

			if projectName != "" && subProjectName != "" {
				if projCfg, ok := cfg.Projects[projectName]; ok {
					if subProjCfg, ok := projCfg.SubProjects[subProjectName]; ok {
						if envOverride, ok := subProjCfg.EnvOverrides[key]; ok {
							if envOverride.Disabled != nil {
								item.Disabled = *envOverride.Disabled
							}
						} else {
							item.Disabled = true
						}
					}
				}
			}

			envs = append(envs, item)
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
