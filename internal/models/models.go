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
	IsGroup   bool      `json:"is_group" db:"is_group"`
	Name      *string   `json:"name" db:"name"` // Для групп
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// WebSocketMessage — формат сообщений в сокетах
type WebSocketMessage struct {
	Type    string      `json:"type"` // "message", "typing", "status", "read"
	Payload interface{} `json:"payload"`
}