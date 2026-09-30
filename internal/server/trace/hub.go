package trace

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// 允许所有来源（生产环境应该限制）
		return true
	},
}

// TraceHub 管理 WebSocket 连接和 Trace 消息转发
type TraceHub struct {
	clients    map[*websocket.Conn]bool
	broadcast  chan []byte
	register   chan *websocket.Conn
	unregister chan *websocket.Conn
	mutex      sync.RWMutex
	subscribed bool
}

// NewTraceHub 创建新的 Trace Hub
func NewTraceHub() *TraceHub {
	return &TraceHub{
		clients:    make(map[*websocket.Conn]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *websocket.Conn),
		unregister: make(chan *websocket.Conn),
	}
}

// Run 运行 Trace Hub
func (h *TraceHub) Run() {
	// 订阅 sys.trace 主题
	go h.subscribeTrace()

	for {
		select {
		case conn := <-h.register:
			h.mutex.Lock()
			h.clients[conn] = true
			h.mutex.Unlock()
			pkglog.Info("WebSocket client connected, total: %d", len(h.clients))

		case conn := <-h.unregister:
			h.mutex.Lock()
			if _, ok := h.clients[conn]; ok {
				delete(h.clients, conn)
				conn.Close()
			}
			h.mutex.Unlock()
			pkglog.Info("WebSocket client disconnected, total: %d", len(h.clients))

		case message := <-h.broadcast:
			h.mutex.RLock()
			for conn := range h.clients {
				conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
					pkglog.Error("Failed to write message to WebSocket: %v", err)
					delete(h.clients, conn)
					conn.Close()
				}
			}
			h.mutex.RUnlock()
		}
	}
}

// subscribeTrace 订阅 sys.trace 主题
func (h *TraceHub) subscribeTrace() {
	js := bus.GetJetStream()
	if js == nil {
		pkglog.Error("JetStream not initialized, cannot subscribe to trace")
		return
	}

	// 订阅 sys.trace.> 主题
	subject := "sys.trace.>"
	queueGroup := "trace-hub"

	pkglog.Info("Subscribing to trace subject: %s", subject)

	if err := js.SubscribeQueue(subject, queueGroup, func(data []byte) error {
		// 转发到所有 WebSocket 客户端
		h.broadcast <- data
		return nil
	}); err != nil {
		pkglog.Error("Failed to subscribe to trace: %v", err)
		return
	}

	h.subscribed = true
	pkglog.Info("Trace subscription active")
}

// HandleWebSocket 处理 WebSocket 连接
func (h *TraceHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		pkglog.Error("Failed to upgrade connection: %v", err)
		return
	}

	h.register <- conn

	// 启动 goroutine 处理客户端消息（心跳等）
	go func() {
		defer func() {
			h.unregister <- conn
		}()

		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					pkglog.Error("WebSocket error: %v", err)
				}
				break
			}

			// 处理客户端消息（例如：订阅特定规则的 Trace）
			var msg map[string]interface{}
			if err := json.Unmarshal(message, &msg); err == nil {
				if action, ok := msg["action"].(string); ok {
					switch action {
					case "ping":
						// 心跳响应
						conn.WriteJSON(map[string]interface{}{
							"action":    "pong",
							"timestamp": time.Now().Unix(),
						})
					case "subscribe":
						// 订阅特定规则（可以扩展）
						pkglog.Debug("Client subscribed to rule: %v", msg["rule_id"])
					}
				}
			}
		}
	}()
}
