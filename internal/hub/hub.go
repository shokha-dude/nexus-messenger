package hub

import (
	"encoding/json"
	"log"
	"sync"
)

type Hub struct {
	// Защита от конкурентного доступа к map
	sync.RWMutex
	// Все подключенные клиенты (онлайн пользователи)
	Clients    map[int]*Client
	// Каналы для управления
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan interface{} // Для массовых рассылок
}

func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[int]*Client),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Broadcast:  make(chan interface{}),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.Lock()
			h.Clients[client.UserID] = client
			h.Unlock()
			log.Printf("User %s (ID: %d) connected. Total online: %d", 
				client.Username, client.UserID, len(h.Clients))
			
			// Оповестить всех о новом пользователе онлайн
			h.notifyStatus(client.UserID, "online")

		case client := <-h.Unregister:
			if _, ok := h.Clients[client.UserID]; ok {
				h.Lock()
				delete(h.Clients, client.UserID)
				h.Unlock()
				close(client.Send)
				log.Printf("User %s disconnected", client.Username)
				
				// Оповестить всех о том, что пользователь оффлайн
				h.notifyStatus(client.UserID, "offline")
			}

		case message := <-h.Broadcast:
			// Отправка всем (например, системное сообщение)
			h.broadcastToAll(message)
		}
	}
}

func (h *Hub) notifyStatus(userID int, status string) {
	// Создаем уведомление о статусе
	statusMsg := map[string]interface{}{
		"type": "status",
		"payload": map[string]interface{}{
			"user_id": userID,
			"status":  status,
		},
	}
	data, _ := json.Marshal(statusMsg)
	
	h.RLock()
	defer h.RUnlock()
	for _, client := range h.Clients {
		// Не отправляем самому пользователю его же статус
		if client.UserID != userID {
			select {
			case client.Send <- data:
			default:
				close(client.Send)
				delete(h.Clients, client.UserID)
			}
		}
	}
}

func (h *Hub) broadcastToAll(message interface{}) {
	data, _ := json.Marshal(message)
	
	h.RLock()
	defer h.RUnlock()
	for _, client := range h.Clients {
		select {
		case client.Send <- data:
		default:
			close(client.Send)
			delete(h.Clients, client.UserID)
		}
	}
}

// SendToUser — отправить сообщение конкретному пользователю
func (h *Hub) SendToUser(userID int, message interface{}) bool {
	h.RLock()
	client, ok := h.Clients[userID]
	h.RUnlock()
	
	if !ok {
		return false // Пользователь не в сети
	}
	
	data, _ := json.Marshal(message)
	select {
	case client.Send <- data:
		return true
	default:
		// Если не можем отправить (канал заблокирован), удаляем клиента
		h.Unregister <- client
		return false
	}
}