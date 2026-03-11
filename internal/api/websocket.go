package api

import (
	"net/http"
	"nexus/internal/hub"
	"nexus/internal/database"
	"nexus/internal/models"
	"log"

	"github.com/gorilla/websocket"
	"github.com/golang-jwt/jwt/v5"
)

// var jwtKey = []byte("supersecretkey") - УДАЛИ ЭТУ СТРОКУ!

type WebSocketHandler struct {
	Hub *hub.Hub
	DB  *database.SQLiteDB
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *WebSocketHandler) HandleConnections(w http.ResponseWriter, r *http.Request) {
	// Аутентификация через токен в query параметре
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		http.Error(w, "Missing token", http.StatusUnauthorized)
		return
	}

	// Валидация JWT
	claims := &models.Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return JwtKey, nil // ИСПОЛЬЗУЕМ ЭКСПОРТИРОВАННУЮ ПЕРЕМЕННУЮ
	})

	if err != nil || !token.Valid {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Upgrade HTTP to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		http.Error(w, "Could not upgrade to websocket", http.StatusInternalServerError)
		return
	}

	// Создаем клиента
	client := &hub.Client{
		Hub:      h.Hub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		UserID:   claims.UserID,
		Username: claims.Username,
	}

	// Регистрируем в хабе
	h.Hub.Register <- client

	// Запускаем горутины
	go client.WritePump()
	go client.ReadPump()
}