package main

import (
	"fmt"
	"log"
	"os"

	"auto-deploy-platform/internal/api"
	"auto-deploy-platform/internal/builder"
	"auto-deploy-platform/internal/config"
	"auto-deploy-platform/internal/db"
	"auto-deploy-platform/internal/deployer"
	"auto-deploy-platform/internal/gitea"
	"auto-deploy-platform/internal/task"

	"github.com/gin-gonic/gin"
)

func main() {
	// Determine config file path
	configPath := "config.yaml"
	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		configPath = envPath
	}

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize SQLite database
	dbPath := "deploy.db"
	if envDB := os.Getenv("DB_PATH"); envDB != "" {
		dbPath = envDB
	}
	database, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Create Gitea client
	giteaCfg := cfg.GetGiteaConfig()
	giteaClient := gitea.NewGiteaClient(giteaCfg.URL, giteaCfg.Token)

	// Create Builder and Deployer
	bldr := builder.NewBuilder()
	dplyr := deployer.NewDeployer()

	// Create TaskManager
	taskMgr := task.NewTaskManager(database, bldr, dplyr, cfg)

	// Create API handler and register routes
	handler := api.NewHandler(giteaClient, cfg, taskMgr)

	// Initialize Gin engine
	r := gin.Default()
	handler.RegisterRoutes(r)

	// Serve frontend static files in production mode
	distDir := "./web/dist"
	if _, err := os.Stat(distDir); err == nil {
		r.Static("/assets", distDir+"/assets")
		r.StaticFile("/", distDir+"/index.html")
		// SPA fallback: serve index.html for any route not matched by API or static assets
		r.NoRoute(func(c *gin.Context) {
			c.File(distDir + "/index.html")
		})
		log.Printf("Serving frontend static files from %s", distDir)
	} else {
		log.Printf("Warning: frontend dist directory %s not found, skipping static file serving", distDir)
	}

	// Determine port
	port := cfg.Server.Port
	if port == 0 {
		port = 8080
	}

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Starting server on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
