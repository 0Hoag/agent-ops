package board

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Invalidation struct {
	Type         string `json:"type"`
	RepositoryID string `json:"repository_id"`
}
type subscriber struct {
	conn *websocket.Conn
	ch   chan Invalidation
}
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[*subscriber]struct{}
}

func NewHub() *Hub { return &Hub{subs: make(map[string]map[*subscriber]struct{})} }
func (h *Hub) Publish(repositoryID string) {
	event := Invalidation{Type: "board.invalidate", RepositoryID: repositoryID}
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub := range h.subs[repositoryID] {
		select {
		case sub.ch <- event:
		default:
			h.removeLocked(repositoryID, sub)
			_ = sub.conn.Close()
		}
	}
}
func (h *Hub) Handle(ctx *gin.Context) {
	repositoryID := ctx.Query("repository_id")
	if _, err := uuid.Parse(repositoryID); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"status": 400, "message": "invalid repository_id"})
		return
	}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
	if err != nil {
		return
	}
	sub := &subscriber{conn: conn, ch: make(chan Invalidation, 16)}
	h.mu.Lock()
	if h.subs[repositoryID] == nil {
		h.subs[repositoryID] = make(map[*subscriber]struct{})
	}
	h.subs[repositoryID][sub] = struct{}{}
	h.mu.Unlock()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	defer func() { h.remove(repositoryID, sub); _ = conn.Close(); <-done }()
	for {
		select {
		case event := <-sub.ch:
			if err := conn.WriteJSON(event); err != nil {
				return
			}
		case <-done:
			return
		}
	}
}
func (h *Hub) remove(repositoryID string, sub *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeLocked(repositoryID, sub)
}
func (h *Hub) removeLocked(repositoryID string, sub *subscriber) {
	if group := h.subs[repositoryID]; group != nil {
		delete(group, sub)
		if len(group) == 0 {
			delete(h.subs, repositoryID)
		}
	}
}
