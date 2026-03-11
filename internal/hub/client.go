package hub

import (
	"log"
	"nexus/internal/models"
	
	"github.com/gorilla/websocket"
)

type Client struct {
	Hub      *Hub
	Conn     *websocket.Conn
	Send     chan []byte // Канал для отправки сообщений этому клиенту
	UserID   int
	Username string
}

// readPump — горутина, которая читает сообщения от клиента
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c // Отписываем клиента при ошибке/закрытии
		c.Conn.Close()
	}()

	for {
		var wsMsg models.WebSocketMessage
		err := c.Conn.ReadJSON(&wsMsg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Обработка разных типов сообщений
		switch wsMsg.Type {
		case "message":
			// Здесь будет логика отправки сообщения в БД и адресату
			c.Hub.Broadcast <- wsMsg.Payload // Пока просто шлем всем
		case "typing":
			// Логика уведомления о печати
		}
	}
}

// writePump — горутина, которая отправляет сообщения клиенту
func (c *Client) WritePump() {
	defer c.Conn.Close()

	for message := range c.Send {
		err := c.Conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			log.Printf("Error writing to client: %v", err)
			break
		}
	}
}