package chat

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type incomingMessage struct {
	Text string `json:"text"`
	User string `json:"user"`
}

func HistoryHandler(hub *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		room := strings.TrimSpace(c.Query("room"))
		if room == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "room is required"})
			return
		}

		c.JSON(http.StatusOK, hub.History(room))
	}
}

func WSHandler(hub *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		room := strings.TrimSpace(c.Query("room"))
		if room == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "room is required"})
			return
		}

		user := strings.TrimSpace(c.Query("user"))
		if user == "" {
			user = "anon"
		}

		ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}

		history := hub.Join(room, ws, user)
		for _, msg := range history {
			_ = ws.WriteJSON(msg)
		}

		for {
			_, payload, err := ws.ReadMessage()
			if err != nil {
				break
			}

			var incoming incomingMessage
			if err := json.Unmarshal(payload, &incoming); err != nil {
				text := strings.TrimSpace(string(payload))
				if text == "" {
					continue
				}
				hub.Broadcast(Message{
					Type: "message",
					Room: room,
					User: hub.User(room, ws),
					Text: text,
					At:   time.Now().UTC(),
				})
				continue
			}

			text := strings.TrimSpace(incoming.Text)
			if text == "" {
				continue
			}

			msgUser := strings.TrimSpace(incoming.User)
			if msgUser == "" {
				msgUser = hub.User(room, ws)
			}

			hub.Broadcast(Message{
				Type: "message",
				Room: room,
				User: msgUser,
				Text: text,
				At:   time.Now().UTC(),
			})
		}

		hub.Leave(room, ws)
	}
}
