package main

import (
	"log"
	"net/http"
	"nexus/internal/api"
	"nexus/internal/database"
	"nexus/internal/hub"
)

func main() {
	// Подключение к SQLite (файл будет создан автоматически)
	connStr := "./nexus.db" // Просто путь к файлу базы данных

	db, err := database.NewSQLiteDB(connStr)
	if err != nil {
		log.Fatal("Failed to connect to DB:", err)
	}
	defer db.DB.Close()

	// Инициализация схемы базы данных
	log.Println("Initializing database schema...")
	if err := db.InitSchema(); err != nil {
		log.Fatal("Failed to initialize database schema:", err)
	}
	log.Println("Database schema initialized successfully!")

	// Создаем Hub и запускаем его менеджер
	mainHub := hub.NewHub()
	go mainHub.Run()

	// Создаем обработчики
	authHandler := &api.AuthHandler{DB: db}
	wsHandler := &api.WebSocketHandler{Hub: mainHub, DB: db}

	// Маршруты
	http.HandleFunc("/register", authHandler.Register)
	http.HandleFunc("/login", authHandler.Login)
	http.HandleFunc("/ws", wsHandler.HandleConnections)

	// Статика для теста
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	log.Println("Nexus server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}