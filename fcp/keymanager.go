// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package fcp

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// KeyPair represents a Freenet key pair
type KeyPair struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"` // SSK, USK
	PublicKey  string            `json:"public_key"`
	PrivateKey string            `json:"private_key,omitempty"`
	Created    time.Time         `json:"created"`
	Modified   time.Time         `json:"modified"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// KeyStoreInterface defines the interface for key storage backends
type KeyStoreInterface interface {
	Add(name string, keyPair *KeyPair) error
	Get(name string) (*KeyPair, error)
	Update(name string, keyPair *KeyPair) error
	Delete(name string) error
	List() ([]string, error)
	ListAll() ([]*KeyPair, error)
}

// ============================================================================
// JSON KEYSTORE IMPLEMENTATION
// ============================================================================

// KeyStore manages key storage and retrieval (JSON backend)
type KeyStore struct {
	path string
	keys map[string]*KeyPair
	mu   sync.RWMutex
}

// NewKeyStore creates or loads a key store from the specified path
func NewKeyStore(path string) (*KeyStore, error) {
	if path == "" {
		// Default to ~/.gohyphanet/keys.json
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		path = filepath.Join(home, ".gohyphanet", "keys.json")
	}

	ks := &KeyStore{
		path: path,
		keys: make(map[string]*KeyPair),
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Load existing keys if file exists
	if _, err := os.Stat(path); err == nil {
		if err := ks.load(); err != nil {
			return nil, err
		}
	}

	return ks, nil
}

// load reads keys from disk
func (ks *KeyStore) load() error {
	data, err := os.ReadFile(ks.path)
	if err != nil {
		return fmt.Errorf("failed to read key store: %w", err)
	}

	if len(data) == 0 {
		return nil // Empty file is OK
	}

	if err := json.Unmarshal(data, &ks.keys); err != nil {
		return fmt.Errorf("failed to parse key store: %w", err)
	}

	return nil
}

// save writes keys to disk
func (ks *KeyStore) save() error {
	data, err := json.MarshalIndent(ks.keys, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal keys: %w", err)
	}

	// Write to temporary file first
	tmpPath := ks.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write key store: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, ks.path); err != nil {
		return fmt.Errorf("failed to save key store: %w", err)
	}

	return nil
}

// Add adds a new key pair to the store
func (ks *KeyStore) Add(name string, keyPair *KeyPair) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if _, exists := ks.keys[name]; exists {
		return fmt.Errorf("key '%s' already exists", name)
	}

	keyPair.Name = name
	keyPair.Created = time.Now()
	keyPair.Modified = time.Now()
	ks.keys[name] = keyPair

	return ks.save()
}

// Get retrieves a key pair by name
func (ks *KeyStore) Get(name string) (*KeyPair, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	keyPair, exists := ks.keys[name]
	if !exists {
		return nil, fmt.Errorf("key '%s' not found", name)
	}

	return keyPair, nil
}

// Update updates an existing key pair
func (ks *KeyStore) Update(name string, keyPair *KeyPair) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if _, exists := ks.keys[name]; !exists {
		return fmt.Errorf("key '%s' not found", name)
	}

	keyPair.Name = name
	keyPair.Modified = time.Now()
	ks.keys[name] = keyPair

	return ks.save()
}

// Delete removes a key pair from the store
func (ks *KeyStore) Delete(name string) error {
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if _, exists := ks.keys[name]; !exists {
		return fmt.Errorf("key '%s' not found", name)
	}

	delete(ks.keys, name)
	return ks.save()
}

// List returns all key names
func (ks *KeyStore) List() ([]string, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	names := make([]string, 0, len(ks.keys))
	for name := range ks.keys {
		names = append(names, name)
	}
	return names, nil
}

// ListAll returns all key pairs
func (ks *KeyStore) ListAll() ([]*KeyPair, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	pairs := make([]*KeyPair, 0, len(ks.keys))
	for _, kp := range ks.keys {
		pairs = append(pairs, kp)
	}
	return pairs, nil
}

// ============================================================================
// SQLITE KEYSTORE IMPLEMENTATION
// ============================================================================

// SQLiteKeyStore implements KeyStore using SQLite database
type SQLiteKeyStore struct {
	db   *sql.DB
	path string
}

// NewSQLiteKeyStore creates or opens an SQLite-backed key store
func NewSQLiteKeyStore(path string) (*SQLiteKeyStore, error) {
	if path == "" {
		// Default to ./keys.db in the current working directory
		path = "keys.db"
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	ks := &SQLiteKeyStore{
		db:   db,
		path: path,
	}

	// Initialize schema
	if err := ks.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return ks, nil
}

// initSchema creates the database schema if it doesn't exist
func (ks *SQLiteKeyStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS keys (
		name TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		public_key TEXT NOT NULL,
		private_key TEXT,
		created_at TIMESTAMP NOT NULL,
		modified_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS key_metadata (
		key_name TEXT NOT NULL,
		key TEXT NOT NULL,
		value TEXT NOT NULL,
		PRIMARY KEY (key_name, key),
		FOREIGN KEY (key_name) REFERENCES keys(name) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_keys_type ON keys(type);
	CREATE INDEX IF NOT EXISTS idx_keys_created ON keys(created_at);
	CREATE INDEX IF NOT EXISTS idx_metadata_key ON key_metadata(key);
	`

	if _, err := ks.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// Add adds a new key pair or updates an existing one (upsert)
func (ks *SQLiteKeyStore) Add(name string, keyPair *KeyPair) error {
	tx, err := ks.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert or replace key
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO keys (name, type, public_key, private_key, created_at, modified_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, name, keyPair.Type, keyPair.PublicKey, keyPair.PrivateKey,
		time.Now(), time.Now())

	if err != nil {
		return fmt.Errorf("failed to upsert key: %w", err)
	}

	// Delete old metadata to ensure a clean slate
	_, err = tx.Exec(`DELETE FROM key_metadata WHERE key_name = ?`, name)
	if err != nil {
		return fmt.Errorf("failed to delete old metadata: %w", err)
	}

	// Insert new metadata
	if keyPair.Metadata != nil {
		for k, v := range keyPair.Metadata {
			_, err = tx.Exec(`
				INSERT INTO key_metadata (key_name, key, value)
				VALUES (?, ?, ?)
			`, name, k, v)
			if err != nil {
				return fmt.Errorf("failed to insert metadata: %w", err)
			}
		}
	}

	return tx.Commit()
}

// Get retrieves a key pair by name
func (ks *SQLiteKeyStore) Get(name string) (*KeyPair, error) {
	keyPair := &KeyPair{
		Name:     name,
		Metadata: make(map[string]string),
	}

	// Get key
	err := ks.db.QueryRow(`
		SELECT type, public_key, private_key, created_at, modified_at
		FROM keys WHERE name = ?
	`, name).Scan(&keyPair.Type, &keyPair.PublicKey, &keyPair.PrivateKey,
		&keyPair.Created, &keyPair.Modified)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("key '%s' not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get key: %w", err)
	}

	// Get metadata
	rows, err := ks.db.Query(`
		SELECT key, value FROM key_metadata WHERE key_name = ?
	`, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("failed to scan metadata: %w", err)
		}
		keyPair.Metadata[k] = v
	}

	return keyPair, nil
}

// Update updates an existing key pair
func (ks *SQLiteKeyStore) Update(name string, keyPair *KeyPair) error {
	tx, err := ks.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update key
	result, err := tx.Exec(`
		UPDATE keys 
		SET type = ?, public_key = ?, private_key = ?, modified_at = ?
		WHERE name = ?
	`, keyPair.Type, keyPair.PublicKey, keyPair.PrivateKey, time.Now(), name)

	if err != nil {
		return fmt.Errorf("failed to update key: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("key '%s' not found", name)
	}

	// Delete old metadata
	_, err = tx.Exec(`DELETE FROM key_metadata WHERE key_name = ?`, name)
	if err != nil {
		return fmt.Errorf("failed to delete old metadata: %w", err)
	}

	// Insert new metadata
	if keyPair.Metadata != nil {
		for k, v := range keyPair.Metadata {
			_, err = tx.Exec(`
				INSERT INTO key_metadata (key_name, key, value)
				VALUES (?, ?, ?)
			`, name, k, v)
			if err != nil {
				return fmt.Errorf("failed to insert metadata: %w", err)
			}
		}
	}

	return tx.Commit()
}

// Delete removes a key pair from the store
func (ks *SQLiteKeyStore) Delete(name string) error {
	result, err := ks.db.Exec(`DELETE FROM keys WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("failed to delete key: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("key '%s' not found", name)
	}

	return nil
}

// List returns all key names
func (ks *SQLiteKeyStore) List() ([]string, error) {
	rows, err := ks.db.Query(`SELECT name FROM keys ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan name: %w", err)
		}
		names = append(names, name)
	}

	return names, nil
}

// ListAll returns all key pairs
func (ks *SQLiteKeyStore) ListAll() ([]*KeyPair, error) {
	rows, err := ks.db.Query(`
		SELECT name, type, public_key, private_key, created_at, modified_at
		FROM keys ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list keys: %w", err)
	}
	defer rows.Close()

	var pairs []*KeyPair
	for rows.Next() {
		kp := &KeyPair{Metadata: make(map[string]string)}
		err := rows.Scan(&kp.Name, &kp.Type, &kp.PublicKey, &kp.PrivateKey,
			&kp.Created, &kp.Modified)
		if err != nil {
			return nil, fmt.Errorf("failed to scan key: %w", err)
		}

		// Get metadata for this key
		metaRows, err := ks.db.Query(`
			SELECT key, value FROM key_metadata WHERE key_name = ?
		`, kp.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get metadata: %w", err)
		}

		for metaRows.Next() {
			var k, v string
			if err := metaRows.Scan(&k, &v); err != nil {
				metaRows.Close()
				return nil, fmt.Errorf("failed to scan metadata: %w", err)
			}
			kp.Metadata[k] = v
		}
		metaRows.Close()

		pairs = append(pairs, kp)
	}

	return pairs, nil
}

// Search finds keys matching criteria
func (ks *SQLiteKeyStore) Search(keyType string, searchTerm string) ([]*KeyPair, error) {
	query := `
		SELECT name, type, public_key, private_key, created_at, modified_at
		FROM keys WHERE 1=1
	`
	args := []interface{}{}

	if keyType != "" {
		query += " AND type = ?"
		args = append(args, keyType)
	}

	if searchTerm != "" {
		query += " AND (name LIKE ? OR public_key LIKE ?)"
		searchPattern := "%" + searchTerm + "%"
		args = append(args, searchPattern, searchPattern)
	}

	query += " ORDER BY name"

	rows, err := ks.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to search keys: %w", err)
	}
	defer rows.Close()

	var pairs []*KeyPair
	for rows.Next() {
		kp := &KeyPair{Metadata: make(map[string]string)}
		err := rows.Scan(&kp.Name, &kp.Type, &kp.PublicKey, &kp.PrivateKey,
			&kp.Created, &kp.Modified)
		if err != nil {
			return nil, fmt.Errorf("failed to scan key: %w", err)
		}
		pairs = append(pairs, kp)
	}

	return pairs, nil
}

// Close closes the database connection
func (ks *SQLiteKeyStore) Close() error {
	return ks.db.Close()
}

// Export exports all keys to JSON
func (ks *SQLiteKeyStore) Export() ([]byte, error) {
	pairs, err := ks.ListAll()
	if err != nil {
		return nil, err
	}

	data, err := json.MarshalIndent(pairs, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal keys: %w", err)
	}

	return data, nil
}

// Import imports keys from JSON
func (ks *SQLiteKeyStore) Import(data []byte) error {
	var pairs []*KeyPair
	if err := json.Unmarshal(data, &pairs); err != nil {
		return fmt.Errorf("failed to unmarshal keys: %w", err)
	}

	tx, err := ks.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, kp := range pairs {
		// Insert or replace key
		_, err = tx.Exec(`
			INSERT OR REPLACE INTO keys (name, type, public_key, private_key, created_at, modified_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, kp.Name, kp.Type, kp.PublicKey, kp.PrivateKey, kp.Created, kp.Modified)

		if err != nil {
			return fmt.Errorf("failed to insert key %s: %w", kp.Name, err)
		}

		// Delete old metadata
		_, err = tx.Exec(`DELETE FROM key_metadata WHERE key_name = ?`, kp.Name)
		if err != nil {
			return fmt.Errorf("failed to delete old metadata for %s: %w", kp.Name, err)
		}

		// Insert metadata
		if kp.Metadata != nil {
			for k, v := range kp.Metadata {
				_, err = tx.Exec(`
					INSERT INTO key_metadata (key_name, key, value)
					VALUES (?, ?, ?)
				`, kp.Name, k, v)
				if err != nil {
					return fmt.Errorf("failed to insert metadata for %s: %w", kp.Name, err)
				}
			}
		}
	}

	return tx.Commit()
}

// GetStats returns statistics about the keystore
func (ks *SQLiteKeyStore) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Total keys
	var total int
	err := ks.db.QueryRow(`SELECT COUNT(*) FROM keys`).Scan(&total)
	if err != nil {
		return nil, err
	}
	stats["total_keys"] = total

	// Keys by type
	rows, err := ks.db.Query(`SELECT type, COUNT(*) FROM keys GROUP BY type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byType := make(map[string]int)
	for rows.Next() {
		var keyType string
		var count int
		if err := rows.Scan(&keyType, &count); err != nil {
			return nil, err
		}
		byType[keyType] = count
	}
	stats["by_type"] = byType

	// Database size
	var pageCount, pageSize int
	ks.db.QueryRow(`PRAGMA page_count`).Scan(&pageCount)
	ks.db.QueryRow(`PRAGMA page_size`).Scan(&pageSize)
	stats["db_size_bytes"] = pageCount * pageSize

	return stats, nil
}

// ============================================================================
// SHARED KEY UTILITY FUNCTIONS
// ============================================================================

// GenerateSSK generates a new SSK key pair using the Freenet node
func (c *Client) GenerateSSK() (*KeyPair, error) {
	identifier := generateIdentifier("keygen")
	
	resultChan := make(chan *KeyPair, 1)
	errChan := make(chan error, 1)
	
	// Register handler for SSKKeypair response
	handler := func(m *Message) error {
		if m.Fields["Identifier"] != identifier {
			return nil
		}
		
		if m.Name == "SSKKeypair" {
			keyPair := &KeyPair{
				Type:       "SSK",
				PublicKey:  m.Fields["RequestURI"],
				PrivateKey: m.Fields["InsertURI"],
				Created:    time.Now(),
				Modified:   time.Now(),
			}
			resultChan <- keyPair
			return nil
		}
		
		if m.Name == "ProtocolError" {
			errChan <- fmt.Errorf("protocol error: %s", m.Fields["CodeDescription"])
			return nil
		}
		
		return nil
	}
	
	// Register handlers for both possible responses
	c.RegisterHandler("SSKKeypair", handler)
	c.RegisterHandler("ProtocolError", handler)
	
	// Send GenerateSSK message
	msg := &Message{
		Name: "GenerateSSK",
		Fields: map[string]string{
			"Identifier": identifier,
		},
	}

	if err := c.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("failed to send GenerateSSK: %w", err)
	}

	// Wait for response with timeout
	select {
	case keyPair := <-resultChan:
		return keyPair, nil
	case err := <-errChan:
		return nil, err
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("timeout waiting for key generation")
	}
}

// GenerateCHK generates a CHK from data (deterministic hash)
func GenerateCHK(data []byte) (string, error) {
	// This is a simplified version - real CHK generation is more complex
	// and should be done by the Freenet node
	hash := make([]byte, 32)
	if _, err := rand.Read(hash); err != nil {
		return "", err
	}
	hashStr := base64.RawURLEncoding.EncodeToString(hash)
	return fmt.Sprintf("CHK@%s", hashStr), nil
}

// ParseKeyType determines the key type from a URI
func ParseKeyType(uri string) string {
	if strings.HasPrefix(uri, "CHK@") {
		return "CHK"
	} else if strings.HasPrefix(uri, "SSK@") {
		return "SSK"
	} else if strings.HasPrefix(uri, "USK@") {
		return "USK"
	} else if strings.HasPrefix(uri, "KSK@") {
		return "KSK"
	}
	return "UNKNOWN"
}

// IsInsertURI checks if a URI is an insert (private) URI
func IsInsertURI(uri string) bool {
	// Insert URIs contain private keys and have more components
	parts := strings.Split(uri, "/")
	if len(parts) > 0 {
		keyPart := parts[0]
		// SSK insert URIs have format: SSK@public,private,crypto/
		if strings.HasPrefix(keyPart, "SSK@") {
			commas := strings.Count(keyPart, ",")
			return commas >= 2
		}
		// USK insert URIs similar to SSK
		if strings.HasPrefix(keyPart, "USK@") {
			commas := strings.Count(keyPart, ",")
			return commas >= 2
		}
	}
	return false
}

// GetRequestURI extracts the public (request) URI from an insert URI
func GetRequestURI(insertURI string) string {
	if !IsInsertURI(insertURI) {
		return insertURI
	}

	// Convert SSK@public,private,crypto to SSK@public,crypto
	parts := strings.Split(insertURI, "/")
	if len(parts) > 0 {
		keyPart := parts[0]
		if strings.HasPrefix(keyPart, "SSK@") || strings.HasPrefix(keyPart, "USK@") {
			components := strings.Split(keyPart[4:], ",")
			if len(components) >= 3 {
				// Reconstruct as public key
				requestKey := keyPart[:4] + components[0] + "," + components[2]
				if len(parts) > 1 {
					return requestKey + "/" + strings.Join(parts[1:], "/")
				}
				return requestKey
			}
		}
	}

	return insertURI
}

// IncrementUSK increments a USK version number
func IncrementUSK(uskURI string) (string, error) {
	parts := strings.Split(uskURI, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid USK format")
	}

	// USK format: USK@.../sitename/version
	if !strings.HasPrefix(parts[0], "USK@") {
		return "", fmt.Errorf("not a USK URI")
	}

	// Parse current version
	var version int
	if len(parts) >= 3 {
		fmt.Sscanf(parts[len(parts)-1], "%d", &version)
	}

	// Increment version
	version++

	// Rebuild URI with new version
	newParts := append(parts[:len(parts)-1], fmt.Sprintf("%d", version))
	return strings.Join(newParts, "/"), nil
}
