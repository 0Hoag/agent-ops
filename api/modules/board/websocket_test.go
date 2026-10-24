package board

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestWebSocketPublishesOnlyToMatchingRepository(t *testing.T) {
	hub := NewHub()
	router := httptest.NewServer(NewControllerWithHub(nil, hub).hubHandler())
	t.Cleanup(router.Close)
	connect := func(id string) *websocket.Conn {
		wsURL := "ws" + router.URL[len("http"):] + "/api/v1/board/ws?repository_id=" + id
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err)
		return conn
	}
	a := "11111111-1111-1111-1111-111111111111"
	b := "22222222-2222-2222-2222-222222222222"
	connA, connB := connect(a), connect(b)
	t.Cleanup(func() { _ = connA.Close(); _ = connB.Close() })
	hub.Publish(a)
	_, payload, err := connA.ReadMessage()
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"board.invalidate","repository_id":"`+a+`"}`, string(payload))
	_ = connB.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err = connB.ReadMessage()
	require.Error(t, err)
}

func TestWebSocketDisconnectRemovesSubscription(t *testing.T) {
	hub := NewHub()
	server := httptest.NewServer(NewControllerWithHub(nil, hub).hubHandler())
	t.Cleanup(server.Close)
	id := "11111111-1111-1111-1111-111111111111"
	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+"/api/v1/board/ws?repository_id="+id, nil)
	require.NoError(t, err)
	hub.mu.Lock()
	require.Len(t, hub.subs[id], 1)
	hub.mu.Unlock()
	require.NoError(t, conn.Close())
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		hub.mu.Lock()
		count := len(hub.subs[id])
		hub.mu.Unlock()
		if count == 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("disconnected websocket subscription was not removed")
}

func (c *Controller) hubHandler() http.Handler {
	router := gin.New()
	NewRoutes(c).Register(router.Group("/api/v1"))
	return router
}
