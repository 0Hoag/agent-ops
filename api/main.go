package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	_ "github.com/0Hoag/agent-ops/api/docs"
	"github.com/0Hoag/agent-ops/api/modules/account"
	"github.com/0Hoag/agent-ops/api/modules/agentrun"
	"github.com/0Hoag/agent-ops/api/modules/auth"
	"github.com/0Hoag/agent-ops/api/modules/board"
	"github.com/0Hoag/agent-ops/api/modules/insight"
	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/modules/repositorysettings"
	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/modules/wave"
	"github.com/0Hoag/agent-ops/api/modules/webhook"
	"github.com/0Hoag/agent-ops/api/pkg/config"
	appcrypto "github.com/0Hoag/agent-ops/api/pkg/crypto"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/0Hoag/agent-ops/api/pkg/handlers"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm/builder"
	_ "github.com/0Hoag/agent-ops/api/pkg/handlers/scm/github"
	_ "github.com/0Hoag/agent-ops/api/pkg/handlers/scm/gitlab"
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
// @securityDefinitions.apikey AgentRunToken
// @in header
// @name X-Agent-Run-Token
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Bearer account token

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
		if err := configured.Repositories.UpsertConfigured(context.Background(), &store.Repository{ID: store.NewUUID(), Provider: repository.Provider, FullName: repository.FullName, OrganizationID: repository.OrganizationID, WebhookSecretEnvRef: repository.WebhookSecretEnv}); err != nil {
			logger.Fatal(fmt.Errorf("configure repository: %w", err))
		}
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID(), middleware.SecurityHeaders())
	router.Use(productionCORS(cfg))
	router.Use(logger.Request(log))
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	registerRoutes(router, database.AsPinger(db), db, cfg)

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	log.Info("api listening", "addr", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal(fmt.Errorf("server stopped: %w", err))
	}
}

func productionCORS(cfg config.Config) gin.HandlerFunc {
	return cors.New(cors.Config{AllowOrigins: cfg.CORSOrigins, AllowMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"}, AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID", "X-Hub-Signature-256", "X-Gitlab-Token", "X-Agent-Run-Token"}, AllowCredentials: false, MaxAge: 12 * time.Hour})
}

func registerRoutes(router *gin.Engine, db database.Pinger, gormDB *gorm.DB, cfg config.Config) {
	registerRoutesWithHub(router, db, gormDB, cfg, nil)
}

func registerRoutesWithHub(router *gin.Engine, db database.Pinger, gormDB *gorm.DB, cfg config.Config, supplied *board.Hub) {
	handlers.NewHealth(db).Register(router)
	configs := map[string][]webhook.Config{}
	for _, r := range cfg.Repositories {
		configs[r.Provider] = append(configs[r.Provider], webhook.Config{Repository: r.FullName, SecretEnv: r.WebhookSecretEnv, TokenEnv: r.TokenEnv, BaseURLEnv: r.BaseURLEnv})
	}
	hub := supplied
	if hub == nil {
		hub = board.NewHub()
	}
	authService := auth.NewService(account.NewRepository(gormDB), cfg.JWTSecret, cfg.JWTExpiration)
	var webhookSealer *appcrypto.Sealer
	if sealer, sealErr := appcrypto.New(cfg.EncryptionKey); sealErr == nil {
		webhookSealer = sealer
	}
	webhook.NewRoutes(webhook.NewController(webhook.NewServiceWithPublisherAndSealer(gormDB, builder.Default(), hub, webhookSealer), configs)).Register(router.Group("/api/v1"))
	private := router.Group("/api/v1", middleware.RequireOrganization(authService))
	if sealer, sealErr := appcrypto.New(cfg.EncryptionKey); sealErr == nil {
		repositorysettings.NewRoutes(repositorysettings.NewController(store.New(gormDB).Repositories, organization.NewRepository(gormDB), sealer, "")).Register(private)
	}
	agentrun.RegisterPriceRoutes(private, agentrun.NewPriceController(agentrun.NewRepository(gormDB), organization.NewRepository(gormDB)))
	board.NewRoutes(board.NewControllerWithHub(board.NewRepository(gormDB), hub)).Register(private)
	wave.NewRoutes(wave.NewController(wave.NewRepository(gormDB), organization.NewRepository(gormDB))).Register(private)
	insight.NewRoutes(insight.NewController(insight.NewRepository(gormDB))).Register(private)
	prices := map[string]agentrun.Price{}
	for model, raw := range cfg.AgentRunModelPrices {
		var p agentrun.Price
		if err := json.Unmarshal(raw, &p); err == nil {
			prices[model] = p
		}
	}
	var configuredRepositories []string
	if len(cfg.Repositories) > 0 {
		configuredRepositories = make([]string, 0, len(cfg.Repositories))
		for _, repository := range cfg.Repositories {
			configuredRepositories = append(configuredRepositories, repository.FullName)
		}
	}
	// Agent ingestion uses its dedicated machine token; it must not require an interactive user session.
	runController := agentrun.NewControllerWithConfiguredRepositories(agentrun.NewRepository(gormDB), cfg.AgentRunToken, prices, configuredRepositories)
	runController.SetExchangeRateVND(cfg.ExchangeRateVNDPerUSD)
	runRoutes := agentrun.NewRoutes(runController)
	runRoutes.Register(router.Group("/api/v1"))
	runRoutes.RegisterPrivate(private)
	runRoutes.RegisterAggregate(private)
	auth.NewRoutes(auth.NewController(authService)).Register(router.Group("/api/v1"))
	organization.NewRoutes(organization.NewController(organization.NewRepository(gormDB), authService)).Register(router.Group("/api/v1"))
}
