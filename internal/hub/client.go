package hub

import (
	"encoding/json"
	"log"
	"nexus/internal/models"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	Hub      *Hub
	Conn     *websocket.Conn
	Send     chan []byte // Канал для отправки сообщений этому клиенту
	UserID   int
	Username string
    lastTyping time.Time // время последнего статуса
    typingMutex sync.Mutex
}

// readPump — горутина, которая читает сообщения от клиента
func (c *Client) ReadPump() {
    defer func() {
        c.Hub.Unregister <- c
        c.Conn.Close()
    }()

    // В ReadPump или в отдельной функции при подключении
    offlineMessages, err := c.Hub.DB.GetOfflineMessages(c.UserID)
    if err == nil && len(offlineMessages) > 0 {
        for _, msg := range offlineMessages {
            var messageData map[string]interface{}
            json.Unmarshal([]byte(msg), &messageData)
            c.SendMessage(messageData)
        }
    }

    for {
        var wsMsg models.WebSocketMessage
        err := c.Conn.ReadJSON(&wsMsg)
        if err != nil {
            break
        }

        switch wsMsg.Type {
        case "create_group":
            // Создание новой группы
            var payload models.CreateGroupPayload
            data, _ := json.Marshal(wsMsg.Payload)
            json.Unmarshal(data, &payload)

            log.Printf("Creating group: %s with members: %v", payload.Name, payload.Members)

            // Создаем группу в БД
            group, err := c.Hub.DB.CreateGroup(payload.Name, c.UserID, payload.Members)
            if err != nil {
                log.Printf("Error creating group: %v", err)
                c.SendMessage(map[string]interface{}{
                    "type": "error",
                    "payload": map[string]interface{}{
                        "message": "Failed to create group: " + err.Error(),
                    },
                })
                continue
            }

            // Получаем полную информацию о группе (уже включает created_by)
            groupInfo, err := c.Hub.DB.GetGroupInfo(group.ID)
            if err != nil {
                log.Printf("Error getting group info: %v", err)
                continue
            }

            // Отправляем подтверждение создателю
            c.SendMessage(map[string]interface{}{
                "type": "group_created",
                "payload": groupInfo,  // теперь включает CreatedBy
            })

            // Уведомляем всех участников о новой группе
            log.Printf("Notifying members: %v", groupInfo.Members)
            for _, member := range groupInfo.Members {
                if member != c.Username { // не отправляем создателю повторно
                    memberID, _ := c.Hub.DB.GetUserByUsername(member)
                    if memberID != nil {
                        notification := map[string]interface{}{
                            "type": "added_to_group",
                            "payload": map[string]interface{}{
                                "group_id":   group.ID,
                                "group_name": group.Name,
                                "added_by":   c.Username,
                            },
                        }
                        log.Printf("Sending notification to %s", member)
                        c.Hub.SendToUser(memberID.ID, notification)
                    }
                }
            }

        case "private_message":
            // Отправка личного сообщения
            var payload models.PrivateMessagePayload
            data, _ := json.Marshal(wsMsg.Payload)
            json.Unmarshal(data, &payload)

            recipient, err := c.Hub.DB.GetUserByUsername(payload.To)
            if err != nil {
                c.SendMessage(map[string]interface{}{
                    "type": "error",
                    "payload": map[string]interface{}{
                        "message": "User not found: " + payload.To,
                    },
                })
                continue
            }

            convID, err := c.Hub.DB.GetOrCreatePrivateConversation(c.UserID, recipient.ID)
            if err != nil {
                log.Printf("Error creating conversation: %v", err)
                c.SendMessage(map[string]interface{}{
                    "type": "error",
                    "payload": map[string]interface{}{
                        "message": "Failed to create conversation",
                    },
                })
                continue
            }

            log.Printf("Saving message to conversation %d", convID)

            msg, err := c.Hub.DB.SaveMessage(convID, c.UserID, payload.Text)
            if err != nil {
                log.Printf("Error saving message: %v", err)
                continue
            }

            // Отправляем только получателю
            outgoingMsg := map[string]interface{}{
                "type": "private_message",
                "payload": map[string]interface{}{
                    "id":              msg.ID,
                    "from":            c.Username,
                    "from_id":         c.UserID,
                    "text":            msg.Text,
                    "created_at":      msg.CreatedAt,
                    "conversation_id": convID,
                    "chat_type":       "private",  // ← добавить тип
                },
            }

            sent := c.Hub.SendToUser(recipient.ID, outgoingMsg)
            log.Printf("Message sent to %s: %v", recipient.Username, sent) // ← добавить

            // Подтверждение отправителю
            confirmMsg := map[string]interface{}{
                "type": "message_sent",
                "payload": map[string]interface{}{
                    "id":          msg.ID,
                    "to":          payload.To,
                    "to_id":       recipient.ID,
                    "text":        msg.Text,
                    "created_at":  msg.CreatedAt,
                    "delivered":   sent,
                },
            }
            c.SendMessage(confirmMsg)

        case "group_message":
            var payload models.GroupMessagePayload
            data, _ := json.Marshal(wsMsg.Payload)
            json.Unmarshal(data, &payload)

            // Проверяем, существует ли группа
            groupInfo, err := c.Hub.DB.GetGroupInfo(payload.ConversationID)
            if err != nil {
                log.Printf("Error getting group info: %v", err)
                c.SendMessage(map[string]interface{}{
                    "type": "error",
                    "payload": map[string]interface{}{
                        "message": "Group not found",
                    },
                })
                continue
            }

            // Сохраняем сообщение
            msg, err := c.Hub.DB.SaveMessage(payload.ConversationID, c.UserID, payload.Text)
            if err != nil {
                log.Printf("Error saving group message: %v", err)
                continue
            }

            // Формируем сообщение для рассылки
            outgoingMsg := map[string]interface{}{
                "type": "group_message",
                "payload": map[string]interface{}{
                    "id":              msg.ID,
                    "from":            c.Username,
                    "from_id":         c.UserID,
                    "text":            msg.Text,
                    "created_at":      msg.CreatedAt,
                    "conversation_id": payload.ConversationID,
                    "group_name":      groupInfo.Name,
                    "chat_type":       "group",
                },
            }

            // Рассылаем всем участникам группы
            delivered := 0
            for _, member := range groupInfo.Members {
                if member != c.Username {
                    memberID, err := c.Hub.DB.GetUserByUsername(member)
                    if err == nil && memberID != nil {
                        if c.Hub.SendToUser(memberID.ID, outgoingMsg) {
                            delivered++
                        }
                    }
                }
            }

            log.Printf("Group message delivered to %d/%d members", delivered, len(groupInfo.Members)-1)

            // Подтверждение отправителю
            c.SendMessage(map[string]interface{}{
                "type": "message_sent",
                "payload": map[string]interface{}{
                    "id":          msg.ID,
                    "text":        msg.Text,
                    "created_at":  msg.CreatedAt,
                    "conversation_id": payload.ConversationID,
                    "is_group":    true,
                    "delivered":   delivered > 0,
                },
            })

        case "get_my_groups":
            // Получение списка групп пользователя
            groups, err := c.Hub.DB.GetUserGroups(c.UserID)
            if err != nil {
                log.Printf("Error getting groups: %v", err)
                continue
            }

            c.SendMessage(map[string]interface{}{
                "type": "my_groups",
                "payload": groups,
            })

        case "get_group_info":
            // Получение информации о конкретной группе
            var req struct {
                ConversationID int `json:"conversation_id"`
            }
            data, _ := json.Marshal(wsMsg.Payload)
            json.Unmarshal(data, &req)

            groupInfo, err := c.Hub.DB.GetGroupInfo(req.ConversationID)
            if err != nil {
                c.SendMessage(map[string]interface{}{
                    "type": "error",
                    "payload": map[string]interface{}{
                        "message": "Group not found",
                    },
                })
                continue
            }

            c.SendMessage(map[string]interface{}{
                "type": "group_info",
                "payload": groupInfo,
            })

        case "get_group_history":
            var req struct {
                ConversationID int `json:"conversation_id"`
                Limit          int `json:"limit"`
            }
            data, _ := json.Marshal(wsMsg.Payload)
            json.Unmarshal(data, &req)

            if req.Limit == 0 {
                req.Limit = 50
            }

            // Проверяем, что пользователь состоит в группе
            groupInfo, err := c.Hub.DB.GetGroupInfo(req.ConversationID)
            if err != nil {
                log.Printf("Error getting group info: %v", err)
                c.SendMessage(map[string]interface{}{
                    "type": "error",
                    "payload": map[string]interface{}{"message": "Group not found"},
                })
                continue
            }

            // Проверяем, является ли пользователь участником
            isMember := false
            for _, member := range groupInfo.Members {
                if member == c.Username {
                    isMember = true
                    break
                }
            }

            if !isMember {
                c.SendMessage(map[string]interface{}{
                    "type": "error",
                    "payload": map[string]interface{}{"message": "You are not a member of this group"},
                })
                continue
            }

            // Получаем сообщения
            messages, err := c.Hub.DB.GetConversationMessages(req.ConversationID, req.Limit, 0)
            if err != nil {
                log.Printf("Error getting messages: %v", err)
                messages = []models.Message{}
            }

            log.Printf("Found %d messages for group %d", len(messages), req.ConversationID)

            c.SendMessage(map[string]interface{}{
                "type": "group_history",
                "payload": map[string]interface{}{
                    "conversation_id": req.ConversationID,
                    "messages":        messages,
                    "group_name":      groupInfo.Name,
                },
            })

        case "add_to_group":
            // Добавление пользователей в группу
            var payload models.AddToGroupPayload
            data, _ := json.Marshal(wsMsg.Payload)
            json.Unmarshal(data, &payload)

            // Добавляем пользователей
            err := c.Hub.DB.AddUsersToGroup(payload.ConversationID, payload.Users)
            if err != nil {
                c.SendMessage(map[string]interface{}{
                    "type": "error",
                    "payload": map[string]interface{}{
                        "message": "Failed to add users: " + err.Error(),
                    },
                })
                continue
            }

            // Получаем обновленную информацию о группе
            groupInfo, err := c.Hub.DB.GetGroupInfo(payload.ConversationID)
            if err != nil {
                log.Printf("Error getting group info: %v", err)
                continue
            }

            // Уведомляем всех участников об обновлении
            for _, member := range groupInfo.Members {
                memberID, _ := c.Hub.DB.GetUserByUsername(member)
                if memberID != nil {
                    notification := map[string]interface{}{
                        "type": "group_updated",
                        "payload": map[string]interface{}{
                            "group_id":   payload.ConversationID,
                            "group_name": groupInfo.Name,
                            "members":    groupInfo.Members,
                        },
                    }
                    c.Hub.SendToUser(memberID.ID, notification)
                }
            }

            // Подтверждение отправителю
            c.SendMessage(map[string]interface{}{
                "type": "users_added",
                "payload": map[string]interface{}{
                    "conversation_id": payload.ConversationID,
                    "added_users":     payload.Users,
                },
            })

        case "get_private_history":
            var req struct {
                WithUser string `json:"with_user"`
                Limit    int    `json:"limit"`
            }
            data, _ := json.Marshal(wsMsg.Payload)
            json.Unmarshal(data, &req)

            if req.Limit == 0 {
                req.Limit = 50
            }

            // Находим собеседника
            otherUser, err := c.Hub.DB.GetUserByUsername(req.WithUser)
            if err != nil {
                c.SendMessage(map[string]interface{}{
                    "type": "error",
                    "payload": map[string]interface{}{"message": "User not found"},
                })
                continue
            }

            // Получаем или создаем личный чат
            convID, err := c.Hub.DB.GetOrCreatePrivateConversation(c.UserID, otherUser.ID)
            if err != nil {
                log.Printf("Error getting conversation: %v", err)
                continue
            }

            log.Printf("Getting messages for conversation %d between %s and %s",
                convID, c.Username, req.WithUser)

            // Получаем сообщения
            messages, err := c.Hub.DB.GetConversationMessages(convID, req.Limit, 0)
            if err != nil {
                log.Printf("Error getting messages: %v", err)
                messages = []models.Message{}
            }

            log.Printf("Found %d messages with %s for conversation %d",
                len(messages), req.WithUser, convID)

            c.SendMessage(map[string]interface{}{
                "type": "private_history",
                "payload": map[string]interface{}{
                    "with_user": req.WithUser,
                    "messages":  messages,
                },
            })

        case "get_online_users":
            // Запрос списка онлайн пользователей
            users := c.Hub.GetOnlineUsers()
            response := map[string]interface{}{
                "type": "online_users",
                "payload": users,
            }
            c.SendMessage(response)

        case "typing":
            var payload models.TypingPayload
            data, _ := json.Marshal(wsMsg.Payload)
            json.Unmarshal(data, &payload)

            log.Printf("Typing event: %+v", payload)

            // Определяем, кому отправлять уведомление
            if payload.InGroup > 0 {
                // Это группа
                go c.handleGroupTyping(payload)
            } else if payload.To != "" {
                // Это личное сообщение
                go c.handlePrivateTyping(payload)
            }
        }
    }
}

// handlePrivateTyping - обработка статуса печати в личке
func (c *Client) handlePrivateTyping(payload models.TypingPayload) {
    c.typingMutex.Lock()
    defer c.typingMutex.Unlock()

    // Не чаще чем раз в секунду
    if time.Since(c.lastTyping) < time.Second {
        return
    }
    c.lastTyping = time.Now()

    // Находим получателя
    recipient, err := c.Hub.DB.GetUserByUsername(payload.To)
    if err != nil {
        log.Printf("Typing: recipient not found: %s", payload.To)
        return
    }

    // Формируем статус
    typingStatus := map[string]interface{}{
        "type": "typing_status",
        "payload": models.TypingStatus{
            From:     c.Username,
            IsTyping: payload.IsTyping,
        },
    }

    // Отправляем получателю
    c.Hub.SendToUser(recipient.ID, typingStatus)
}

// handleGroupTyping - обработка статуса печати в группе
func (c *Client) handleGroupTyping(payload models.TypingPayload) {
    // Получаем информацию о группе
    groupInfo, err := c.Hub.DB.GetGroupInfo(payload.InGroup)
    if err != nil {
        log.Printf("Typing: group not found: %d", payload.InGroup)
        return
    }

    // Формируем статус
    typingStatus := map[string]interface{}{
        "type": "typing_status",
        "payload": models.TypingStatus{
            From:     c.Username,
            IsTyping: payload.IsTyping,
            InGroup:  payload.InGroup,
        },
    }

    // Рассылаем всем участникам группы, кроме отправителя
    for _, member := range groupInfo.Members {
        if member != c.Username {
            memberID, err := c.Hub.DB.GetUserByUsername(member)
            if err == nil && memberID != nil {
                c.Hub.SendToUser(memberID.ID, typingStatus)
            }
        }
    }
}

// SendMessage — удобный метод для отправки сообщения клиенту
func (c *Client) SendMessage(msg interface{}) {
    data, err := json.Marshal(msg)
    if err != nil {
        log.Printf("Failed to marshal message: %v", err)
        return
    }

    select {
    case c.Send <- data:
    default:
        log.Printf("Client send channel full for user %s", c.Username)
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