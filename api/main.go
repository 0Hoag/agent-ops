package main

import (
	"context"
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
		log.Error("open database", "error", err)
		return
	}
	defer database.Close(db)

	if err := database.RunMigrations(context.Background(), db); err != nil {
		log.Error("run migrations", "error", err)
		return
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(logger.Request(log))
	registerRoutes(router, database.AsPinger(db))

	server := &http.Server{Addr: cfg.HTTPAddr, Handler: router, ReadHeaderTimeout: 5 * time.Second}
	log.Info("api listening", "addr", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("server stopped", "error", err)
	}
}

func registerRoutes(router *gin.Engine, db database.Pinger) {
	handlers.NewHealth(db).Register(router)
}
