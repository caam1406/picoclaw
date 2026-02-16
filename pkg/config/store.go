package config

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/zalando/go-keyring"
	_ "modernc.org/sqlite"
)

const (
	configTableName  = "app_config"
	configRowID      = 1
	keyringService   = "picoclaw"
	keyringConfigKey = "config-master-key"
)

func DefaultConfigDBPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "picoclaw.db")
}

func LegacyConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "config.json")
}

func loadConfigFromStore(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigDBPath()
	}

	store, err := newConfigStore(path)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, exists, err := store.load(ctx)
	if err != nil {
		return nil, err
	}

	if !exists {
		if store.driver == "postgres" {
			sqlitePath := path
			if strings.TrimSpace(sqlitePath) == "" {
				sqlitePath = DefaultConfigDBPath()
			}
			if migratedCfg, migrated, err := migrateConfigFromSQLiteFile(ctx, store, sqlitePath); err != nil {
				return nil, err
			} else if migrated {
				return migratedCfg, nil
			}
		}

		legacyPath := LegacyConfigPath()
		if legacyCfg, migrated, err := migrateLegacyConfig(ctx, store, legacyPath); err != nil {
			return nil, err
		} else if migrated {
			return legacyCfg, nil
		}

		cfg = DefaultConfig()
		if err := store.save(ctx, cfg); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func saveConfigToStore(path string, cfg *Config) error {
	if path == "" {
		path = DefaultConfigDBPath()
	}

	store, err := newConfigStore(path)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return store.save(ctx, cfg)
}

type configStore struct {
	driver string
	dsn    string
}

func newConfigStore(path string) (*configStore, error) {
	driver, dsn, err := resolveConfigStoreTarget(path)
	if err != nil {
		return nil, err
	}

	if driver == "sqlite" {
		dir := filepath.Dir(dsn)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	return &configStore{driver: driver, dsn: dsn}, nil
}

func resolveConfigStoreTarget(path string) (string, string, error) {
	configDBURL := firstNonEmptyEnv(
		"PICOCLAW_CONFIG_DATABASE_URL",
	)

	if configDBURL == "" {
		storageType := strings.ToLower(strings.TrimSpace(os.Getenv("PICOCLAW_STORAGE_TYPE")))
		if storageType == "postgres" {
			configDBURL = firstNonEmptyEnv("PICOCLAW_STORAGE_DATABASE_URL")
		}
	}

	if configDBURL == "" {
		pgUser := strings.TrimSpace(os.Getenv("POSTGRES_USER"))
		pgPass := strings.TrimSpace(os.Getenv("POSTGRES_PASSWORD"))
		pgDB := strings.TrimSpace(os.Getenv("POSTGRES_DB"))
		if pgUser != "" && pgPass != "" && pgDB != "" {
			configDBURL = fmt.Sprintf("postgres://%s:%s@postgres:5432/%s?sslmode=disable", pgUser, pgPass, pgDB)
		}
	}

	if configDBURL != "" {
		return "postgres", ensurePostgresSSLMode(configDBURL), nil
	}

	if path == "" {
		path = DefaultConfigDBPath()
	}
	if strings.TrimSpace(path) == "" {
		return "", "", errors.New("config DB path is empty")
	}
	return "sqlite", path, nil
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" {
			return value
		}
	}
	return ""
}

func ensurePostgresSSLMode(url string) string {
	if strings.Contains(url, "sslmode=") {
		return url
	}
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	return url + sep + "sslmode=disable"
}

func (s *configStore) openDB() (*sql.DB, error) {
	return sql.Open(s.driver, s.dsn)
}

func (s *configStore) load(ctx context.Context) (*Config, bool, error) {
	db, err := s.openDB()
	if err != nil {
		return nil, false, err
	}
	defer db.Close()

	if err := ensureConfigSchema(ctx, db); err != nil {
		return nil, false, err
	}

	var ciphertext, nonce []byte
	var version int
	query := fmt.Sprintf("SELECT ciphertext, nonce, version FROM %s WHERE id = ?", configTableName)
	if s.driver == "postgres" {
		query = fmt.Sprintf("SELECT ciphertext, nonce, version FROM %s WHERE id = $1", configTableName)
	}
	row := db.QueryRowContext(ctx, query, configRowID)

	if err := row.Scan(&ciphertext, &nonce, &version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}

	key, err := getMasterKey()
	if err != nil {
		return nil, false, err
	}

	plaintext, err := decryptConfig(key, ciphertext, nonce)
	if err != nil {
		return nil, false, err
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(plaintext, cfg); err != nil {
		return nil, false, err
	}

	return cfg, true, nil
}

func (s *configStore) save(ctx context.Context, cfg *Config) error {
	db, err := s.openDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if err := ensureConfigSchema(ctx, db); err != nil {
		return err
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	key, err := getMasterKey()
	if err != nil {
		return err
	}

	ciphertext, nonce, err := encryptConfig(key, data)
	if err != nil {
		return err
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (id, ciphertext, nonce, version, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			ciphertext = excluded.ciphertext,
			nonce = excluded.nonce,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, configTableName)
	if s.driver == "postgres" {
		query = fmt.Sprintf(`
		INSERT INTO %s (id, ciphertext, nonce, version, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT(id) DO UPDATE SET
			ciphertext = excluded.ciphertext,
			nonce = excluded.nonce,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, configTableName)
	}

	_, err = db.ExecContext(ctx, query, configRowID, ciphertext, nonce, 1, time.Now().UTC())

	return err
}

func ensureConfigSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			ciphertext BLOB NOT NULL,
			nonce BLOB NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL
		)
	`, configTableName))
	if err == nil {
		return nil
	}

	// PostgreSQL schema variant (BYTEA / TIMESTAMPTZ)
	_, err = db.ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			ciphertext BYTEA NOT NULL,
			nonce BYTEA NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL
		)
	`, configTableName))
	return err
}

func migrateConfigFromSQLiteFile(ctx context.Context, target *configStore, sqlitePath string) (*Config, bool, error) {
	if strings.TrimSpace(sqlitePath) == "" {
		return nil, false, nil
	}
	if _, err := os.Stat(sqlitePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}

	sqliteDB, err := sql.Open("sqlite", sqlitePath)
	if err != nil {
		return nil, false, err
	}
	defer sqliteDB.Close()

	var tableName string
	if err := sqliteDB.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
		configTableName,
	).Scan(&tableName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}

	var ciphertext, nonce []byte
	var version int
	row := sqliteDB.QueryRowContext(ctx,
		fmt.Sprintf("SELECT ciphertext, nonce, version FROM %s WHERE id = ?", configTableName),
		configRowID,
	)
	if err := row.Scan(&ciphertext, &nonce, &version); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}

	key, err := getMasterKey()
	if err != nil {
		return nil, false, err
	}
	plaintext, err := decryptConfig(key, ciphertext, nonce)
	if err != nil {
		return nil, false, err
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(plaintext, cfg); err != nil {
		return nil, false, err
	}

	if err := target.save(ctx, cfg); err != nil {
		return nil, false, err
	}
	return cfg, true, nil
}

func getMasterKey() ([]byte, error) {
	encoded, err := keyring.Get(keyringService, keyringConfigKey)
	if err == nil {
		key, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, err
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("invalid config master key length")
		}
		return key, nil
	}

	// First fallback: read key from local file (headless/container-safe).
	if key, fileErr := loadMasterKeyFromFallbackFile(); fileErr == nil {
		return key, nil
	}

	// Key not found or keyring unavailable: generate a new key.
	key := make([]byte, 32)
	if _, readErr := rand.Read(key); readErr != nil {
		return nil, readErr
	}
	encoded = base64.StdEncoding.EncodeToString(key)

	// Best-effort write to system keyring; if it fails, persist to fallback file.
	if setErr := keyring.Set(keyringService, keyringConfigKey, encoded); setErr != nil {
		return saveMasterKeyToFallbackFile(key)
	}

	return key, nil
}

func fallbackMasterKeyPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", ".config-master-key")
}

func loadMasterKeyFromFallbackFile() ([]byte, error) {
	path := fallbackMasterKeyPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	encoded := string(data)
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid fallback config master key length")
	}
	return key, nil
}

func saveMasterKeyToFallbackFile(key []byte) ([]byte, error) {
	path := fallbackMasterKeyPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	if err := os.WriteFile(path, []byte(encoded), 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func encryptConfig(key, plaintext []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

func decryptConfig(key, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, nil)
}
