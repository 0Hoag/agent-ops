package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/config"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/0Hoag/agent-ops/api/pkg/handlers"
	"github.com/0Hoag/agent-ops/api/pkg/logger"
	"github.com/gin-gonic/gin"
)

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

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(logger.Request(log))
	registerRoutes(router, database.AsPinger(db))

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	log.Info("api listening", "addr", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal(fmt.Errorf("server stopped: %w", err))
	}
}

func registerRoutes(router *gin.Engine, db database.Pinger) {
	handlers.NewHealth(db).Register(router)
}
