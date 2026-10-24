package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	_ "github.com/0Hoag/agent-ops/api/docs"
	"github.com/0Hoag/agent-ops/api/modules/agentrun"
	"github.com/0Hoag/agent-ops/api/modules/board"
	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/modules/webhook"
	"github.com/0Hoag/agent-ops/api/pkg/config"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/0Hoag/agent-ops/api/pkg/handlers"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm/builder"
	_ "github.com/0Hoag/agent-ops/api/pkg/handlers/scm/github"
	"github.com/0Hoag/agent-ops/api/pkg/logger"
	"github.com/0Hoag/agent-ops/api/pkg/middleware"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// @title Agent Ops API
// @version 1.0
// @description API for repository synchronization and signed webhooks.
// @host localhost:8080
// @BasePath /

func main() {
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal(err)
	}
	log := logger.New(cfg.LogLevel)
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		logger.Fatal(fmt.Errorf("open database: %w", err))
	}
	defer database.Close(db)

	if err := database.RunMigrations(context.Background(), db); err != nil {
		logger.Fatal(fmt.Errorf("run migrations: %w", err))
	}
	configured := store.New(db)
	for _, repository := range cfg.Repositories {
		if err := configured.Repositories.Upsert(context.Background(), &store.Repository{ID: store.NewUUID(), Provider: repository.Provider, FullName: repository.FullName, WebhookSecretEnvRef: repository.WebhookSecretEnv}); err != nil {
			logger.Fatal(fmt.Errorf("configure repository: %w", err))
		}
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID(), middleware.SecurityHeaders())
	router.Use(cors.New(cors.Config{AllowOrigins: cfg.CORSOrigins, AllowMethods: []string{"GET", "POST", "OPTIONS"}, AllowHeaders: []string{"Origin", "Content-Type", "Accept", "X-Request-ID", "X-Hub-Signature-256", "X-Gitlab-Token", "X-Agent-Run-Token"}, AllowCredentials: false, MaxAge: 12 * time.Hour}))
	router.Use(logger.Request(log))
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	registerRoutes(router, database.AsPinger(db), db, cfg)

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	log.Info("api listening", "addr", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal(fmt.Errorf("server stopped: %w", err))
	}
}

func registerRoutes(router *gin.Engine, db database.Pinger, gormDB *gorm.DB, cfg config.Config) {
	handlers.NewHealth(db).Register(router)
	configs := map[string][]webhook.Config{}
	for _, r := range cfg.Repositories {
		configs[r.Provider] = append(configs[r.Provider], webhook.Config{Repository: r.FullName, SecretEnv: r.WebhookSecretEnv, TokenEnv: r.TokenEnv, BaseURLEnv: r.BaseURLEnv})
	}
	hub := board.NewHub()
	webhook.NewRoutes(webhook.NewController(webhook.NewServiceWithPublisher(gormDB, builder.Default(), hub), configs)).Register(router.Group("/api/v1"))
	board.NewRoutes(board.NewControllerWithHub(board.NewRepository(gormDB), hub)).Register(router.Group("/api/v1"))
	prices := map[string]agentrun.Price{}
	for model, raw := range cfg.AgentRunModelPrices {
		var p agentrun.Price
		if err := json.Unmarshal(raw, &p); err == nil {
			prices[model] = p
		}
	}
	agentrun.NewRoutes(agentrun.NewController(agentrun.NewRepository(gormDB), cfg.AgentRunToken, prices)).Register(router.Group("/api/v1"))
}
