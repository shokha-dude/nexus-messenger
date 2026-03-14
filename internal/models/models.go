package models

import (
	"time"
	"github.com/golang-jwt/jwt/v5"
)

// User — модель пользователя в БД
type User struct {
	ID        int       `json:"id" db:"id"`
	Username  string    `json:"username" db:"username"`
	Password  string    `json:"-" db:"password_hash"` // Хеш пароля, не показываем в JSON
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Claims — данные для JWT токена
type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Message — сообщение (для БД и пересылки)
type Message struct {
	ID             int       `json:"id" db:"id"`
	ConversationID int       `json:"conversation_id" db:"conversation_id"`
	SenderID       int       `json:"sender_id" db:"sender_id"`
	SenderUsername string    `json:"sender_username"` // Денормализовано для удобства
	Text           string    `json:"text" db:"text"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// Conversation — чат (диалог или группа)
type Conversation struct {
    ID        int       `json:"id" db:"id"`
    IsGroup   bool      `json:"is_group" db:"is_group"`     // true для групп, false для лички
    Name      *string   `json:"name" db:"name"`              // название группы (для лички null)
    CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ConversationParticipant — участник чата
type ConversationParticipant struct {
    ConversationID int       `json:"conversation_id" db:"conversation_id"`
    UserID         int       `json:"user_id" db:"user_id"`
    JoinedAt       time.Time `json:"joined_at" db:"joined_at"`
}

// Новый тип сообщения для WebSocket
type PrivateMessagePayload struct {
    Text         string `json:"text"`          // текст сообщения
    To           string `json:"to"`             // кому (username)
    ConversationID int    `json:"conversation_id,omitempty"` // ID чата (если уже есть)
}

// WebSocketMessage — формат сообщений в сокетах
type WebSocketMessage struct {
	Type    string      `json:"type"` // "message", "typing", "status", "read"
	Payload interface{} `json:"payload"`
}

// CreateGroupPayload — запрос на создание группы
type CreateGroupPayload struct {
    Name        string   `json:"name"`         // Название группы
    Members     []string `json:"members"`      // Список участников (username)
}

// GroupMessagePayload — сообщение в группу
type GroupMessagePayload struct {
    Text           string `json:"text"`                     // текст сообщения
    ConversationID int    `json:"conversation_id"`          // ID группы
}

// AddToGroupPayload — добавление пользователей в группу
type AddToGroupPayload struct {
    ConversationID int      `json:"conversation_id"`        // ID группы
    Users          []string `json:"users"`                  // кого добавить (username)
}

// GroupInfo — информация о группе для клиента
type GroupInfo struct {
    ID        int       `json:"id"`
    Name      string    `json:"name"`
    Members   []string  `json:"members"`     // список участников
    CreatedAt time.Time `json:"created_at"`
    CreatedBy string    `json:"created_by"`  // ИМЯ создателя (исправлено)
}