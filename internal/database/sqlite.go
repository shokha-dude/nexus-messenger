package database

import (
	"database/sql"
	"fmt"
	"log"
	"nexus/internal/models"

	_ "modernc.org/sqlite" // ИЗМЕНЕНО: другой драйвер
	"golang.org/x/crypto/bcrypt"
)

type SQLiteDB struct {
	DB *sql.DB
}

func NewSQLiteDB(dbPath string) (*SQLiteDB, error) {
	// Для modernc.org/sqlite нужно добавить параметр
	db, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("sql.Open error: %w", err)
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("db.Ping error: %w", err)
	}

	log.Println("Successfully connected to SQLite database!")
	return &SQLiteDB{DB: db}, nil
}

// Остальной код без изменений...

func (s *SQLiteDB) InitSchema() error {
	log.Println("Initializing SQLite schema...")

	// SQLite имеет немного другой синтаксис, чем PostgreSQL
	queries := []string{
		// Таблица пользователей (SERIAL в SQLite - это AUTOINCREMENT)
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Таблица чатов
		`CREATE TABLE IF NOT EXISTS conversations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			is_group BOOLEAN DEFAULT 0,
			name TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// Таблица участников чата
		`CREATE TABLE IF NOT EXISTS conversation_participants (
			conversation_id INTEGER,
			user_id INTEGER,
			joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (conversation_id, user_id),
			FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,

		// Таблица сообщений
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER,
			sender_id INTEGER,
			text TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
			FOREIGN KEY (sender_id) REFERENCES users(id) ON DELETE SET NULL
		)`,

		// Индексы для производительности
		`CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_participants_user_id ON conversation_participants(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_users_username ON users(username)`,
	}

	for i, query := range queries {
		log.Printf("Executing query %d...", i)
		_, err := s.DB.Exec(query)
		if err != nil {
			return fmt.Errorf("error executing query %d: %w\nQuery: %s", i, err, query)
		}
	}

	// Проверим, создалась ли таблица users
	var tableName string
	err := s.DB.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='users'").Scan(&tableName)
	if err != nil {
    	if err == sql.ErrNoRows {
        	log.Println("✗ Table 'users' was NOT created")
    	} else {
        	log.Printf("Error checking if users table exists: %v", err)
    	}
	} else {
    	log.Println("✓ Table 'users' created successfully")
	}
	log.Println("SQLite schema initialized successfully!")
	return nil
}

func (s *SQLiteDB) CreateUser(username, password string) (*models.User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	var user models.User
	query := `INSERT INTO users (username, password_hash) VALUES (?, ?) RETURNING id, username, created_at`
	err = s.DB.QueryRow(query, username, string(hashedPassword)).Scan(&user.ID, &user.Username, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *SQLiteDB) GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	var passwordHash string
	query := `SELECT id, username, password_hash, created_at FROM users WHERE username = ?`
	err := s.DB.QueryRow(query, username).Scan(&user.ID, &user.Username, &passwordHash, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	user.Password = passwordHash
	return &user, nil
}

func (s *SQLiteDB) SaveMessage(conversationID, senderID int, text string) (*models.Message, error) {
	var msg models.Message
	query := `
		INSERT INTO messages (conversation_id, sender_id, text) 
		VALUES (?, ?, ?) 
		RETURNING id, conversation_id, sender_id, text, created_at
	`
	err := s.DB.QueryRow(query, conversationID, senderID, text).Scan(
		&msg.ID, &msg.ConversationID, &msg.SenderID, &msg.Text, &msg.CreatedAt)
	if err != nil {
		return nil, err
	}

	// Подтягиваем username отправителя
	_ = s.DB.QueryRow(`SELECT username FROM users WHERE id = ?`, senderID).Scan(&msg.SenderUsername)
	return &msg, nil
}