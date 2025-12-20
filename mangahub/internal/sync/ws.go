package sync

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // OK for demo; restrict in production
	},
}

func WSHandler(hub *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}

		hub.AddWS(ws)
		log.Println("[ws] client connected")

		// Optional welcome message
		_ = ws.WriteMessage(
			websocket.TextMessage,
			[]byte(`{"type":"welcome","transport":"websocket"}`+"\n"),
		)

		// Keep connection alive (ignore incoming messages)
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				break
			}
		}

		hub.RemoveWS(ws)
		log.Println("[ws] client disconnected")
	}
}
