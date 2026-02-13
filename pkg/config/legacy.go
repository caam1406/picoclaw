package config

import (
	"context"
	"encoding/json"
	"errors"
	"os"
)

func LoadConfigFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func migrateLegacyConfig(ctx context.Context, store *configStore, legacyPath string) (*Config, bool, error) {
	if legacyPath == "" {
		return nil, false, nil
	}

	if _, err := os.Stat(legacyPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}

	cfg, err := LoadConfigFromFile(legacyPath)
	if err != nil {
		return nil, false, err
	}

	if err := store.save(ctx, cfg); err != nil {
		return nil, false, err
	}

	backupPath := legacyPath + ".bak"
	if err := os.Rename(legacyPath, backupPath); err != nil {
		_ = os.WriteFile(legacyPath+".migrated", []byte("config migrated to DB"), 0644)
	}

	return cfg, true, nil
}
