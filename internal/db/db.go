package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/sudoku-ui/panel/internal/models"
	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, err
	}

	return db, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS clients (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		private_key TEXT NOT NULL UNIQUE,
		user_hash TEXT NOT NULL,
		enable INTEGER NOT NULL DEFAULT 1,
		remark TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	_, err := d.conn.Exec(schema)
	return err
}

// ========== Clients ==========

func (d *DB) CreateClient(c *models.Client) error {
	now := time.Now().UTC()
	c.CreatedAt = now
	c.UpdatedAt = now

	res, err := d.conn.Exec(
		`INSERT INTO clients (name, private_key, user_hash, enable, remark, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.PrivateKey, c.UserHash, boolToInt(c.Enable), c.Remark, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = id
	return nil
}

func (d *DB) ListClients() ([]models.Client, error) {
	rows, err := d.conn.Query(
		`SELECT id, name, private_key, user_hash, enable, remark, created_at, updated_at
		 FROM clients ORDER BY id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []models.Client
	for rows.Next() {
		var c models.Client
		var enableInt int
		if err := rows.Scan(&c.ID, &c.Name, &c.PrivateKey, &c.UserHash, &enableInt, &c.Remark, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Enable = enableInt == 1
		list = append(list, c)
	}
	return list, rows.Err()
}

func (d *DB) GetClient(id int64) (*models.Client, error) {
	var c models.Client
	var enableInt int
	err := d.conn.QueryRow(
		`SELECT id, name, private_key, user_hash, enable, remark, created_at, updated_at
		 FROM clients WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.PrivateKey, &c.UserHash, &enableInt, &c.Remark, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("client not found")
	}
	if err != nil {
		return nil, err
	}
	c.Enable = enableInt == 1
	return &c, nil
}

func (d *DB) DeleteClient(id int64) error {
	res, err := d.conn.Exec(`DELETE FROM clients WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("client not found")
	}
	return nil
}

func (d *DB) UpdateClient(c *models.Client) error {
	c.UpdatedAt = time.Now().UTC()
	_, err := d.conn.Exec(
		`UPDATE clients SET name = ?, enable = ?, remark = ?, updated_at = ? WHERE id = ?`,
		c.Name, boolToInt(c.Enable), c.Remark, c.UpdatedAt, c.ID,
	)
	return err
}

// ========== Settings ==========

func (d *DB) SetSetting(key, value string) error {
	_, err := d.conn.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

func (d *DB) GetSetting(key string) (string, error) {
	var value string
	err := d.conn.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
