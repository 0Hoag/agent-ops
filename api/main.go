package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	_ "github.com/0Hoag/agent-ops/api/docs"
	"github.com/0Hoag/agent-ops/api/modules/account"
	"github.com/0Hoag/agent-ops/api/modules/agentrun"
	"github.com/0Hoag/agent-ops/api/modules/alert"
	"github.com/0Hoag/agent-ops/api/modules/auth"
	"github.com/0Hoag/agent-ops/api/modules/board"
	"github.com/0Hoag/agent-ops/api/modules/budget"
	"github.com/0Hoag/agent-ops/api/modules/insight"
	"github.com/0Hoag/agent-ops/api/modules/organization"
	"github.com/0Hoag/agent-ops/api/modules/repositorysettings"
	"github.com/0Hoag/agent-ops/api/modules/repositorysync"
	"github.com/0Hoag/agent-ops/api/modules/store"
	"github.com/0Hoag/agent-ops/api/modules/wave"
	"github.com/0Hoag/agent-ops/api/modules/webhook"
	"github.com/0Hoag/agent-ops/api/modules/work"
	"github.com/0Hoag/agent-ops/api/modules/worker"
	"github.com/0Hoag/agent-ops/api/pkg/config"
	appcrypto "github.com/0Hoag/agent-ops/api/pkg/crypto"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/0Hoag/agent-ops/api/pkg/handlers"
	"github.com/0Hoag/agent-ops/api/pkg/handlers/scm/builder"
	_ "github.com/0Hoag/agent-ops/api/pkg/handlers/scm/github"
	_ "github.com/0Hoag/agent-ops/api/pkg/handlers/scm/gitlab"
	"github.com/0Hoag/agent-ops/api/pkg/logger"
	"github.com/0Hoag/agent-ops/api/pkg/metrics"
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
	sealer, _ := appcrypto.New(cfg.EncryptionKey)
	for _, repository := range cfg.Repositories {
		item, err := configuredRepository(repository, sealer)
		if err != nil {
			logger.Fatal(fmt.Errorf("configure repository: %w", err))
		}
		if err := configured.Repositories.UpsertConfigured(context.Background(), item); err != nil {
			logger.Fatal(fmt.Errorf("configure repository: %w", err))
		}
	}
	syncService := repositorysync.New(configured, sealer)
	go func() {
		ticker := time.NewTicker(cfg.SyncInterval)
		defer ticker.Stop()
		for range ticker.C {
			runScheduledSync(context.Background(), log, syncService)
		}
	}()

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID(), middleware.SecurityHeaders())
	router.Use(productionCORS(cfg))
	router.Use(logger.Request(log))
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	registerRoutesWithSync(router, database.AsPinger(db), db, cfg, syncService)

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	log.Info("api listening", "addr", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal(fmt.Errorf("server stopped: %w", err))
	}
}

func configuredRepository(repository config.RepositoryConfig, sealer *appcrypto.Sealer) (*store.Repository, error) {
	item := &store.Repository{ID: store.NewUUID(), Provider: repository.Provider, FullName: repository.FullName, OrganizationID: repository.OrganizationID, WebhookSecretEnvRef: repository.WebhookSecretEnv, GitLabBaseURL: os.Getenv(repository.BaseURLEnv)}
	for _, credential := range []struct {
		env   string
		store func(string)
		name  string
	}{{repository.TokenEnv, func(value string) { item.BackfillTokenCiphertext = value }, "token"}, {repository.WebhookSecretEnv, func(value string) { item.WebhookSecretCiphertext = value }, "webhook secret"}} {
		value := os.Getenv(credential.env)
		if credential.env == "" || value == "" {
			continue
		}
		if sealer == nil {
			return nil, fmt.Errorf("repository %s credential requires repository encryption", credential.name)
		}
		sealed, err := sealer.Seal(value)
		if err != nil {
			return nil, fmt.Errorf("seal repository %s: %w", credential.name, err)
		}
		credential.store(sealed)
	}
	return item, nil
}

type repositorySyncer interface {
	SyncAll(context.Context) error
}

func runScheduledSync(ctx context.Context, log *slog.Logger, syncer repositorySyncer) {
	if syncer.SyncAll(ctx) != nil {
		log.Error("scheduled repository sync failed")
	}
}

func productionCORS(cfg config.Config) gin.HandlerFunc {
	return cors.New(cors.Config{AllowOrigins: cfg.CORSOrigins, AllowMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"}, AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Request-ID", "X-Hub-Signature-256", "X-Gitlab-Token", "X-Agent-Run-Token"}, AllowCredentials: false, MaxAge: 12 * time.Hour})
}

func registerRoutes(router *gin.Engine, db database.Pinger, gormDB *gorm.DB, cfg config.Config) {
	registerRoutesWithRuntime(router, db, gormDB, cfg, nil, true, nil)
}
func registerRoutesWithSync(router *gin.Engine, db database.Pinger, gormDB *gorm.DB, cfg config.Config, syncService *repositorysync.Service) {
	registerRoutesWithRuntime(router, db, gormDB, cfg, nil, true, syncService)
}

func registerRoutesWithHub(router *gin.Engine, db database.Pinger, gormDB *gorm.DB, cfg config.Config, supplied *board.Hub) {
	registerRoutesWithRuntime(router, db, gormDB, cfg, supplied, false, nil)
}

func registerRoutesWithRuntime(router *gin.Engine, db database.Pinger, gormDB *gorm.DB, cfg config.Config, supplied *board.Hub, startRuntime bool, syncService *repositorysync.Service) {
	handlers.NewHealth(db).Register(router)
	serviceMetrics := metrics.New(nil)
	router.Use(serviceMetrics.GinMiddleware())
	router.GET("/metrics", gin.WrapH(serviceMetrics.Handler()))
	configs := map[string][]webhook.Config{}
	for _, r := range cfg.Repositories {
		configs[r.Provider] = append(configs[r.Provider], webhook.Config{Repository: r.FullName, SecretEnv: r.WebhookSecretEnv, TokenEnv: r.TokenEnv, BaseURLEnv: r.BaseURLEnv})
	}
	hub := supplied
	if hub == nil {
		hub = board.NewHub()
	}
	hub.SetQueueObserver(serviceMetrics)
	authService := auth.NewService(account.NewRepository(gormDB), cfg.JWTSecret, cfg.JWTExpiration)
	var webhookSealer *appcrypto.Sealer
	if sealer, sealErr := appcrypto.New(cfg.EncryptionKey); sealErr == nil {
		webhookSealer = sealer
	}
	webhookService := webhook.NewServiceWithPublisherAndSealer(gormDB, builder.Default(), hub, webhookSealer)
	webhookService.SetMetrics(serviceMetrics, nil)
	webhook.NewRoutes(webhook.NewController(webhookService, configs)).Register(router.Group("/api/v1"))
	private := router.Group("/api/v1", middleware.RequireOrganization(authService))
	if sealer, sealErr := appcrypto.New(cfg.EncryptionKey); sealErr == nil {
		repositorysettings.NewRoutes(repositorysettings.NewController(store.New(gormDB).Repositories, organization.NewRepository(gormDB), sealer, "", syncService)).Register(private)
	}
	agentrun.RegisterPriceRoutes(private, agentrun.NewPriceController(agentrun.NewRepository(gormDB), organization.NewRepository(gormDB)))
	board.NewRoutes(board.NewControllerWithHub(board.NewRepository(gormDB), hub)).Register(private)
	budget.NewRoutes(budget.NewController(budget.NewRepository(gormDB), organization.NewRepository(gormDB))).Register(private)
	wave.NewRoutes(wave.NewController(wave.NewRepository(gormDB), organization.NewRepository(gormDB))).Register(private)
	if execution, err := worker.New(worker.Config{QueueSize: cfg.WorkerQueueSize, Concurrency: cfg.WorkerConcurrency, ScriptRoots: cfg.WorkerScriptRoots, WorktreeRoots: cfg.WorkerWorktreeRoots, Scripts: cfg.WorkerScripts}); err == nil {
		worker.NewRoutes(worker.NewController(execution, organization.NewRepository(gormDB), worker.NewAuditRepository(gormDB))).Register(private)
	}
	insight.NewRoutes(insight.NewController(insight.NewRepository(gormDB))).Register(private)
	work.NewRoutes(work.NewController(work.NewRepository(gormDB))).Register(private)
	alert.NewRoutes(alert.NewController(alert.NewRepository(gormDB), organization.NewRepository(gormDB))).Register(private)
	if startRuntime && cfg.DatabaseURL != "" {
		alert.NewRuntime(gormDB, 0, 100).Start(context.Background())
	}
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
