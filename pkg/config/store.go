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
	"time"

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
	path string
}

func newConfigStore(path string) (*configStore, error) {
	if path == "" {
		return nil, errors.New("config DB path is empty")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	return &configStore{path: path}, nil
}

func (s *configStore) load(ctx context.Context) (*Config, bool, error) {
	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		return nil, false, err
	}
	defer db.Close()

	if err := ensureConfigSchema(ctx, db); err != nil {
		return nil, false, err
	}

	var ciphertext, nonce []byte
	var version int
	row := db.QueryRowContext(ctx,
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

	return cfg, true, nil
}

func (s *configStore) save(ctx context.Context, cfg *Config) error {
	db, err := sql.Open("sqlite", s.path)
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

	_, err = db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (id, ciphertext, nonce, version, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			ciphertext = excluded.ciphertext,
			nonce = excluded.nonce,
			version = excluded.version,
			updated_at = excluded.updated_at
	`, configTableName),
		configRowID,
		ciphertext,
		nonce,
		1,
		time.Now().UTC().Format(time.RFC3339),
	)

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
	return err
}

func getMasterKey() ([]byte, error) {
	encoded, err := keyring.Get(keyringService, keyringConfigKey)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			key := make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				return nil, err
			}
			encoded = base64.StdEncoding.EncodeToString(key)
			if err := keyring.Set(keyringService, keyringConfigKey, encoded); err != nil {
				return nil, err
			}
			return key, nil
		}
		return nil, err
	}

	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("invalid config master key length")
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
