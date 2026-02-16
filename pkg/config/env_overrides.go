package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// applyEnvOverrides applies selected runtime environment variables into config.
// It returns true when any value changed so callers can persist updated config.
func applyEnvOverrides(cfg *Config) bool {
	if cfg == nil {
		return false
	}

	changed := false

	setString := func(dst *string, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		if *dst != value {
			*dst = value
			changed = true
		}
	}
	setInt := func(dst *int, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return
		}
		if *dst != parsed {
			*dst = parsed
			changed = true
		}
	}
	setBool := func(dst *bool, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return
		}
		if *dst != parsed {
			*dst = parsed
			changed = true
		}
	}

	env := func(keys ...string) string {
		for _, key := range keys {
			if value := strings.TrimSpace(os.Getenv(key)); value != "" {
				return value
			}
		}
		return ""
	}

	setString(&cfg.Storage.Type, env("PICOCLAW_STORAGE_TYPE"))
	setString(&cfg.Storage.DatabaseURL, env("PICOCLAW_STORAGE_DATABASE_URL", "PICOCLAW_CONFIG_DATABASE_URL"))
	setString(&cfg.Storage.FilePath, env("PICOCLAW_STORAGE_FILE_PATH"))
	setBool(&cfg.Storage.SSLEnabled, env("PICOCLAW_STORAGE_SSL_ENABLED"))

	// If storage type is postgres but no database URL was resolved yet,
	// build one from individual POSTGRES_* env vars (matches resolveConfigStoreTarget logic).
	if strings.EqualFold(cfg.Storage.Type, "postgres") && strings.TrimSpace(cfg.Storage.DatabaseURL) == "" {
		pgUser := strings.TrimSpace(os.Getenv("POSTGRES_USER"))
		pgPass := strings.TrimSpace(os.Getenv("POSTGRES_PASSWORD"))
		pgDB := strings.TrimSpace(os.Getenv("POSTGRES_DB"))
		pgHost := strings.TrimSpace(os.Getenv("POSTGRES_HOST"))
		if pgHost == "" {
			pgHost = "postgres"
		}
		if pgUser != "" && pgPass != "" && pgDB != "" {
			built := fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=disable", pgUser, pgPass, pgHost, pgDB)
			setString(&cfg.Storage.DatabaseURL, built)
		}
	}

	setString(&cfg.Dashboard.Token, env("PICOCLAW_DASHBOARD_TOKEN", "DASHBOARD_TOKEN"))
	setString(&cfg.Dashboard.Host, env("PICOCLAW_DASHBOARD_HOST"))
	setInt(&cfg.Dashboard.Port, env("PICOCLAW_DASHBOARD_PORT"))
	setBool(&cfg.Dashboard.Enabled, env("PICOCLAW_DASHBOARD_ENABLED"))

	setString(&cfg.Providers.OpenRouter.APIKey, env("PICOCLAW_PROVIDERS_OPENROUTER_API_KEY", "OPENROUTER_API_KEY"))
	setString(&cfg.Providers.OpenRouter.APIBase, env("PICOCLAW_PROVIDERS_OPENROUTER_API_BASE"))

	setString(&cfg.Gateway.Host, env("PICOCLAW_GATEWAY_HOST"))
	setInt(&cfg.Gateway.Port, env("PICOCLAW_GATEWAY_PORT"))

	return changed
}

