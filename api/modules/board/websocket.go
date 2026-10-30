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
	mu       sync.Mutex
	subs     map[string]map[*subscriber]struct{}
	observer QueueObserver
}
type QueueObserver interface{ SetQueueDepth(string, int) }

func NewHub() *Hub { return &Hub{subs: make(map[string]map[*subscriber]struct{})} }
func (h *Hub) SetQueueObserver(observer QueueObserver) {
	h.mu.Lock()
	h.observer = observer
	h.mu.Unlock()
}

// SubscriberCount reports the currently registered subscribers for a repository.
// It lets callers coordinate with the asynchronous websocket handshake without
// sending an application-level readiness message.
func (h *Hub) SubscriberCount(repositoryID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[repositoryID])
}

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
	h.observeQueuesLocked(repositoryID)
}

// Handle upgrades the request to a board invalidation websocket.
// @Summary Board invalidation websocket handshake
// @Produce json
// @Param repository_id query string true "Repository UUID" format(uuid)
// @Success 101 {string} string "Switching Protocols"
// @Failure 400 {object} response.ErrorResponse
// @Router /api/v1/board/ws [get]
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
	h.observeQueuesLocked(repositoryID)
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
			h.mu.Lock()
			h.observeQueuesLocked(repositoryID)
			h.mu.Unlock()
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
	h.observeQueuesLocked(repositoryID)
}
func (h *Hub) observeQueuesLocked(repositoryID string) {
	if h.observer == nil {
		return
	}
	depth := 0
	for sub := range h.subs[repositoryID] {
		depth += len(sub.ch)
	}
	h.observer.SetQueueDepth(repositoryID, depth)
}
