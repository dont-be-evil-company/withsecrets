package vaultsync

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// Cache is the desktop ciphertext cache. It is not the legacy provider cache.
type Cache struct {
	db *sql.DB
}

// Open creates or opens a cache database at path.
func Open(path string) (*Cache, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS items (
		instance_id TEXT NOT NULL,
		account_id TEXT NOT NULL,
		vault_id TEXT NOT NULL,
		item_id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		ciphertext BLOB NOT NULL,
		PRIMARY KEY (instance_id, account_id, item_id)
	)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Cache{db: db}, nil
}

// Close closes the database.
func (c *Cache) Close() error { return c.db.Close() }

// Put stores ciphertext. There is no plaintext column.
func (c *Cache) Put(instance, account, vault, item string, revision int64, ciphertext []byte) error {
	_, err := c.db.Exec(`INSERT INTO items (instance_id, account_id, vault_id, item_id, revision, ciphertext)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (instance_id, account_id, item_id) DO UPDATE SET
			vault_id = excluded.vault_id,
			revision = excluded.revision,
			ciphertext = excluded.ciphertext`,
		instance, account, vault, item, revision, ciphertext)
	return err
}

// Get returns one ciphertext row.
func (c *Cache) Get(instance, account, item string) (vault string, revision int64, ciphertext []byte, err error) {
	err = c.db.QueryRow(`SELECT vault_id, revision, ciphertext FROM items WHERE instance_id = ? AND account_id = ? AND item_id = ?`,
		instance, account, item).Scan(&vault, &revision, &ciphertext)
	return vault, revision, ciphertext, err
}

// ClearAccount removes every row for one account on one server.
func (c *Cache) ClearAccount(instance, account string) error {
	_, err := c.db.Exec(`DELETE FROM items WHERE instance_id = ? AND account_id = ?`, instance, account)
	return err
}
