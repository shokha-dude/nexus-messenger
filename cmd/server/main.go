// package main

// import (
// 	"log"
// 	"net/http"
// 	"nexus/internal/api"
// 	"nexus/internal/database"
// 	"nexus/internal/hub"
// 	"os"
// 	"os/exec"
// )

// func init() {
//     // Принудительно устанавливаем UTF-8
//     os.Setenv("LANG", "ru_RU.UTF-8")
//     os.Setenv("LC_ALL", "ru_RU.UTF-8")

//     // Для Windows
//     if os.Getenv("OS") == "Windows_NT" {
//         // Выполняем chcp 65001 через команду
//         exec.Command("cmd", "/c", "chcp", "65001").Run()
//     }
// }

// func main() {
// 	// Подключение к SQLite (файл будет создан автоматически)
// 	connStr := "./nexus.db" // Просто путь к файлу базы данных

// 	db, err := database.NewSQLiteDB(connStr)
// 	if err != nil {
// 		log.Fatal("Failed to connect to DB:", err)
// 	}
// 	defer db.DB.Close()

// 	// Инициализация схемы базы данных
// 	log.Println("Initializing database schema...")
// 	if err := db.InitSchema(); err != nil {
// 		log.Fatal("Failed to initialize database schema:", err)
// 	}
// 	log.Println("Database schema initialized successfully!")

// 	// Создаем Hub и запускаем его менеджер
// 	mainHub := hub.NewHub()
// 	mainHub.DB = db  // Важно: передаем базу данных
// 	go mainHub.Run()

// 	// Создаем обработчики
// 	authHandler := &api.AuthHandler{DB: db}
// 	wsHandler := &api.WebSocketHandler{Hub: mainHub, DB: db}

// 	// Добавь эту строку перед маршрутами
// 	http.Handle("/web/", http.StripPrefix("/web/", http.FileServer(http.Dir("web"))))

// 	// Маршруты
// 	http.HandleFunc("/register", authHandler.Register)
// 	http.HandleFunc("/login", authHandler.Login)
// 	http.HandleFunc("/ws", wsHandler.HandleConnections)

// 	// Вместо старого /
// 	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
//     	http.ServeFile(w, r, "web/index.html")
// 	})

// 	log.Println("Nexus server started on :8080")
// 	log.Fatal(http.ListenAndServe(":8080", nil))
// }
package main

import (
    "log"
    "net/http"
    "nexus/internal/api"
    "nexus/internal/database"
    "nexus/internal/hub"
)

func main() {
    // Подключение к SQLite
    connStr := "./nexus.db"

    db, err := database.NewSQLiteDB(connStr)
    if err != nil {
        log.Fatal("Failed to connect to DB:", err)
    }
    defer db.DB.Close()

    // Инициализация схемы
    log.Println("Initializing database schema...")
    if err := db.InitSchema(); err != nil {
        log.Fatal("Failed to initialize database schema:", err)
    }

    // Создаем Hub
    mainHub := hub.NewHub()
    mainHub.DB = db
    go mainHub.Run()

    // Создаем обработчики
    authHandler := &api.AuthHandler{DB: db}
    wsHandler := &api.WebSocketHandler{Hub: mainHub, DB: db}

    // ВАЖНО: Раздача статических файлов из папки web
    fs := http.FileServer(http.Dir("web"))
    http.Handle("/web/", http.StripPrefix("/web/", fs))

    // Главная страница
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        http.ServeFile(w, r, "web/index.html")
    })

    // API маршруты
    http.HandleFunc("/register", authHandler.Register)
    http.HandleFunc("/login", authHandler.Login)
    http.HandleFunc("/ws", wsHandler.HandleConnections)

    log.Println("Nexus server started on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}