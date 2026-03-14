package database

import (
	"database/sql"
	"fmt"
	"log"
	"nexus/internal/models"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

type SQLiteDB struct {
	DB *sql.DB
}

func NewSQLiteDB(dbPath string) (*SQLiteDB, error) {
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

func (s *SQLiteDB) InitSchema() error {
	log.Println("Initializing SQLite schema...")

	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            username TEXT UNIQUE NOT NULL,
            password_hash TEXT NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )`,
		`CREATE TABLE IF NOT EXISTS conversations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            is_group BOOLEAN DEFAULT 0,
            name TEXT,
            created_by INTEGER,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            FOREIGN KEY (created_by) REFERENCES users(id)
        )`,
		`CREATE TABLE IF NOT EXISTS conversation_participants (
			conversation_id INTEGER,
			user_id INTEGER,
			joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (conversation_id, user_id),
			FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id INTEGER,
			sender_id INTEGER,
			text TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE,
			FOREIGN KEY (sender_id) REFERENCES users(id) ON DELETE SET NULL
		)`,
		`CREATE TABLE IF NOT EXISTS offline_messages (
    		id INTEGER PRIMARY KEY AUTOINCREMENT,
    		user_id INTEGER NOT NULL,
    		message TEXT NOT NULL,
    		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    		FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
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

    // Проверим, существует ли такой conversation
    var exists bool
    err := s.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM conversations WHERE id = ?)", conversationID).Scan(&exists)
    if err != nil {
        return nil, err
    }
    if !exists {
        return nil, fmt.Errorf("conversation %d does not exist", conversationID)
    }

    query := `
        INSERT INTO messages (conversation_id, sender_id, text)
        VALUES (?, ?, ?)
        RETURNING id, conversation_id, sender_id, text, created_at
    `
    err = s.DB.QueryRow(query, conversationID, senderID, text).Scan(
        &msg.ID, &msg.ConversationID, &msg.SenderID, &msg.Text, &msg.CreatedAt,
    )
    if err != nil {
        return nil, err
    }

    err = s.DB.QueryRow(`SELECT username FROM users WHERE id = ?`, senderID).Scan(&msg.SenderUsername)
    if err != nil {
        msg.SenderUsername = "unknown"
    }

    log.Printf("Saved message %d in conversation %d", msg.ID, conversationID)
    return &msg, nil
}

func (s *SQLiteDB) GetOrCreatePrivateConversation(user1ID, user2ID int) (int, error) {
    tx, err := s.DB.Begin()
    if err != nil {
        return 0, err
    }
    defer tx.Rollback()

    // Сначала проверяем существующий чат - ИСПРАВЛЕННЫЙ ЗАПРОС
    var convID int
    query := `
        SELECT c.id
        FROM conversations c
        WHERE c.is_group = 0
        AND EXISTS (
            SELECT 1 FROM conversation_participants
            WHERE conversation_id = c.id AND user_id = ?
        )
        AND EXISTS (
            SELECT 1 FROM conversation_participants
            WHERE conversation_id = c.id AND user_id = ?
        )
        AND (
            SELECT COUNT(*) FROM conversation_participants
            WHERE conversation_id = c.id
        ) = 2
    `
    err = tx.QueryRow(query, user1ID, user2ID).Scan(&convID)

    if err == nil {
        // Чат уже существует
        tx.Commit()
        log.Printf("Found existing conversation %d between %d and %d", convID, user1ID, user2ID)
        return convID, nil
    }

    if err != sql.ErrNoRows {
        return 0, err
    }

    log.Printf("Creating new conversation between %d and %d", user1ID, user2ID)

    // Создаем новый чат
    result, err := tx.Exec(`INSERT INTO conversations (is_group) VALUES (0)`)
    if err != nil {
        return 0, err
    }

    newConvID, err := result.LastInsertId()
    if err != nil {
        return 0, err
    }

    // Добавляем обоих участников
    _, err = tx.Exec(
        `INSERT INTO conversation_participants (conversation_id, user_id) VALUES (?, ?), (?, ?)`,
        newConvID, user1ID, newConvID, user2ID,
    )
    if err != nil {
        return 0, err
    }

    tx.Commit()
    log.Printf("Created new conversation %d", newConvID)
    return int(newConvID), nil
}

func (s *SQLiteDB) GetConversationMessages(conversationID, limit, offset int) ([]models.Message, error) {
    // Сначала проверим, есть ли вообще сообщения в этом чате
    var count int
    err := s.DB.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", conversationID).Scan(&count)
    if err != nil {
        log.Printf("Error counting messages: %v", err)
    } else {
        log.Printf("Total messages in conversation %d: %d", conversationID, count)
    }

    query := `
        SELECT m.id, m.conversation_id, m.sender_id, u.username, m.text, m.created_at
        FROM messages m
        JOIN users u ON m.sender_id = u.id
        WHERE m.conversation_id = ?
        ORDER BY m.created_at ASC
        LIMIT ? OFFSET ?
    `
    rows, err := s.DB.Query(query, conversationID, limit, offset)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var messages []models.Message
    for rows.Next() {
        var msg models.Message
        err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.SenderID,
                        &msg.SenderUsername, &msg.Text, &msg.CreatedAt)
        if err != nil {
            return nil, err
        }
        messages = append(messages, msg)
    }

    log.Printf("GetConversationMessages found %d messages for conversation %d", len(messages), conversationID)
    return messages, nil
}

func (s *SQLiteDB) GetUserConversations(userID int) ([]models.Conversation, error) {
	query := `
		SELECT c.id, c.is_group, c.name, c.created_at
		FROM conversations c
		JOIN conversation_participants cp ON c.id = cp.conversation_id
		WHERE cp.user_id = ?
		ORDER BY (
			SELECT MAX(created_at) FROM messages WHERE conversation_id = c.id
		) DESC
	`
	rows, err := s.DB.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []models.Conversation
	for rows.Next() {
		var conv models.Conversation
		err := rows.Scan(&conv.ID, &conv.IsGroup, &conv.Name, &conv.CreatedAt)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, conv)
	}
	return conversations, nil
}

// Создание группового чата
func (s *SQLiteDB) CreateGroup(name string, creatorID int, memberUsernames []string) (*models.Conversation, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. Создаем запись в conversations с указанием создателя
	result, err := tx.Exec(
		`INSERT INTO conversations (is_group, name, created_by) VALUES (1, ?, ?)`, // ← добавили created_by
		name, creatorID,
	)
	if err != nil {
		return nil, err
	}

	convID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// 2. Получаем ID всех участников (включая создателя)
	var allMembers []int
	allMembers = append(allMembers, creatorID)
	log.Printf("Adding creator ID: %d", creatorID)
	log.Printf("All members: %v", allMembers)

	for _, username := range memberUsernames {
		var userID int
		err := tx.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&userID)
		if err != nil {
			return nil, fmt.Errorf("user not found: %s", username)
		}
		allMembers = append(allMembers, userID)
	}

	// 3. Добавляем всех участников
	for _, userID := range allMembers {
		_, err = tx.Exec(
			`INSERT INTO conversation_participants (conversation_id, user_id) VALUES (?, ?)`,
			convID, userID,
		)
		if err != nil {
			return nil, err
		}
	}

	tx.Commit()

	return &models.Conversation{
		ID:        int(convID),
		IsGroup:   true,
		Name:      &name,
		CreatedAt: time.Now(),
	}, nil
}

// Получение информации о группе
func (s *SQLiteDB) GetGroupInfo(conversationID int) (*models.GroupInfo, error) {
	var group models.GroupInfo
	group.ID = conversationID

	// Получаем название группы и ID создателя
	var createdByID int
	err := s.DB.QueryRow(
		`SELECT name, created_at, created_by FROM conversations WHERE id = ? AND is_group = 1`,
		conversationID,
	).Scan(&group.Name, &group.CreatedAt, &createdByID)
	if err != nil {
		return nil, err
	}

	// Получаем имя создателя
	err = s.DB.QueryRow(`SELECT username FROM users WHERE id = ?`, createdByID).Scan(&group.CreatedBy)
	if err != nil {
		group.CreatedBy = "unknown" // на случай, если пользователь удален
	}

	// Получаем всех участников
	rows, err := s.DB.Query(`
        SELECT u.username
        FROM conversation_participants cp
        JOIN users u ON cp.user_id = u.id
        WHERE cp.conversation_id = ?
    `, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var username string
		rows.Scan(&username)
		members = append(members, username)
	}
	group.Members = members

	return &group, nil
}

// Добавление пользователей в группу
func (s *SQLiteDB) AddUsersToGroup(conversationID int, usernames []string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, username := range usernames {
		var userID int
		err := tx.QueryRow(`SELECT id FROM users WHERE username = ?`, username).Scan(&userID)
		if err != nil {
			return fmt.Errorf("user not found: %s", username)
		}

		_, err = tx.Exec(
			`INSERT OR IGNORE INTO conversation_participants (conversation_id, user_id) VALUES (?, ?)`,
			conversationID, userID,
		)
		if err != nil {
			return err
		}
	}

	tx.Commit()
	return nil
}

// Получение всех групп пользователя
func (s *SQLiteDB) GetUserGroups(userID int) ([]models.Conversation, error) {
	rows, err := s.DB.Query(`
        SELECT c.id, c.name, c.created_at, u.username as created_by
        FROM conversations c
        JOIN conversation_participants cp ON c.id = cp.conversation_id
        LEFT JOIN users u ON c.created_by = u.id
        WHERE cp.user_id = ? AND c.is_group = 1
        ORDER BY (
            SELECT MAX(created_at) FROM messages WHERE conversation_id = c.id
        ) DESC
    `, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []models.Conversation
	for rows.Next() {
		var group models.Conversation
		group.IsGroup = true
		var createdBy sql.NullString
		err := rows.Scan(&group.ID, &group.Name, &group.CreatedAt, &createdBy)
		if err != nil {
			return nil, err
		}
		// Можно добавить поле CreatedBy в Conversation, если нужно
		groups = append(groups, group)
	}
	return groups, nil
}

// SaveOfflineMessage - сохраняет сообщение для офлайн пользователя
func (s *SQLiteDB) SaveOfflineMessage(userID int, message []byte) error {
    // Можно создать отдельную таблицу для офлайн сообщений
    // или просто сохранять в messages с флагом delivered=false
    _, err := s.DB.Exec(
        `INSERT INTO offline_messages (user_id, message, created_at) VALUES (?, ?, CURRENT_TIMESTAMP)`,
        userID, message,
    )
    return err
}

// GetOfflineMessages - получает все офлайн сообщения для пользователя
func (s *SQLiteDB) GetOfflineMessages(userID int) ([]string, error) {
    rows, err := s.DB.Query(
        `SELECT message FROM offline_messages WHERE user_id = ? ORDER BY created_at ASC`,
        userID,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var messages []string
    for rows.Next() {
        var msg string
        rows.Scan(&msg)
        messages = append(messages, msg)
    }

    // Удаляем после получения
    s.DB.Exec(`DELETE FROM offline_messages WHERE user_id = ?`, userID)

    return messages, nil
}