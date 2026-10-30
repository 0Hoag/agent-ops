package worker

import (
	"net/http"

	"github.com/0Hoag/agent-ops/api/pkg/response"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type logMessage struct {
	Type string `json:"type"`
	Line string `json:"line,omitempty"`
}

// Logs streams launcher process stdout/stderr, not .agent-runs transcript JSONL.
// @Summary Worker job log websocket
// @Produce json
// @Param worker_id path string true "Worker job ID"
// @Success 101 {string} string "Switching Protocols"
// @Failure 401,403,404 {object} response.ErrorResponse
// @Router /api/v1/workers/{worker_id}/logs [get]
func (c *Controller) Logs(ctx *gin.Context) {
	org, ok := c.access(ctx, "logs")
	if !ok {
		return
	}
	history, lines, unsubscribe, err := c.executor.SubscribeLogs(ctx.Param("worker_id"), org)
	if err != nil {
		response.NotFound(ctx, "worker not found")
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		unsubscribe()
		return
	}
	defer func() { unsubscribe(); _ = conn.Close() }()
	for _, line := range history {
		if err := conn.WriteJSON(logMessage{Type: "worker.log", Line: line}); err != nil {
			return
		}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	for {
		select {
		case line, open := <-lines:
			if !open {
				_ = conn.WriteJSON(logMessage{Type: "worker.completed"})
				return
			}
			if err := conn.WriteJSON(logMessage{Type: "worker.log", Line: line}); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
