package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/storage"
)

// migrateDataCommand migrates data from file-based storage to PostgreSQL
func migrateDataCommand() {
	fmt.Println("🔄 PicoClaw Data Migration Tool")
	fmt.Println("================================")
	fmt.Println()

	// Load configuration
	configPath := getConfigPath()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("❌ Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Determine source and destination
	var sourceType, destType string
	var sourceConfig, destConfig storage.Config

	if cfg.Storage.Type == "postgres" {
		// Migrating TO postgres, source is file
		sourceType = "file"
		destType = "postgres"

		sourceConfig = storage.Config{
			Type:     "file",
			FilePath: cfg.WorkspacePath(),
		}

		destConfig = storage.Config{
			Type:        "postgres",
			DatabaseURL: cfg.Storage.DatabaseURL,
		}
	} else {
		// Export FROM postgres to file
		sourceType = "postgres"
		destType = "file"

		sourceConfig = storage.Config{
			Type:        "postgres",
			DatabaseURL: cfg.Storage.DatabaseURL,
		}

		destConfig = storage.Config{
			Type:     "file",
			FilePath: cfg.WorkspacePath(),
		}
	}

	fmt.Printf("📁 Source: %s\n", sourceType)
	fmt.Printf("📁 Destination: %s\n", destType)
	fmt.Println()

	// Confirm migration
	fmt.Print("⚠️  This will migrate all data. Continue? (yes/no): ")
	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "yes" {
		fmt.Println("❌ Migration cancelled")
		return
	}

	// Create source storage
	fmt.Printf("🔌 Connecting to source (%s)...\n", sourceType)
	sourceStore, err := storage.NewStorage(sourceConfig)
	if err != nil {
		fmt.Printf("❌ Error creating source storage: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if err := sourceStore.Connect(ctx); err != nil {
		fmt.Printf("❌ Error connecting to source: %v\n", err)
		os.Exit(1)
	}
	defer sourceStore.Close()

	// Create destination storage
	fmt.Printf("🔌 Connecting to destination (%s)...\n", destType)
	destStore, err := storage.NewStorage(destConfig)
	if err != nil {
		fmt.Printf("❌ Error creating destination storage: %v\n", err)
		os.Exit(1)
	}

	if err := destStore.Connect(ctx); err != nil {
		fmt.Printf("❌ Error connecting to destination: %v\n", err)
		os.Exit(1)
	}
	defer destStore.Close()

	// Migrate sessions
	fmt.Println()
	fmt.Println("📦 Migrating sessions...")
	if err := migrateSessions(ctx, sourceStore, destStore); err != nil {
		fmt.Printf("❌ Error migrating sessions: %v\n", err)
		os.Exit(1)
	}

	// Migrate contacts
	fmt.Println()
	fmt.Println("📦 Migrating contacts...")
	if err := migrateContacts(ctx, sourceStore, destStore); err != nil {
		fmt.Printf("❌ Error migrating contacts: %v\n", err)
		os.Exit(1)
	}

	// Migrate cron jobs
	fmt.Println()
	fmt.Println("📦 Migrating cron jobs...")
	if err := migrateCronJobs(ctx, sourceStore, destStore); err != nil {
		fmt.Printf("❌ Error migrating cron jobs: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("✅ Migration completed successfully!")
	fmt.Println()
	fmt.Println("⚠️  Remember to:")
	fmt.Printf("   1. Update storage.type to '%s' in config.json\n", destType)
	fmt.Println("   2. Restart PicoClaw for changes to take effect")
}

func migrateSessions(ctx context.Context, source, dest storage.Storage) error {
	// List all sessions from source
	sessionInfos, err := source.Sessions().List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	fmt.Printf("   Found %d sessions\n", len(sessionInfos))

	// Migrate each session
	for i, info := range sessionInfos {
		fmt.Printf("   [%d/%d] Migrating session: %s\n", i+1, len(sessionInfos), info.Key)

		// Get full session from source
		session, err := source.Sessions().Get(ctx, info.Key)
		if err != nil {
			return fmt.Errorf("failed to get session %s: %w", info.Key, err)
		}

		// Save to destination
		if err := dest.Sessions().Save(ctx, session); err != nil {
			return fmt.Errorf("failed to save session %s: %w", info.Key, err)
		}
	}

	fmt.Printf("   ✅ Migrated %d sessions\n", len(sessionInfos))
	return nil
}

func migrateContacts(ctx context.Context, source, dest storage.Storage) error {
	// List all contacts from source
	contacts, err := source.Contacts().List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list contacts: %w", err)
	}

	fmt.Printf("   Found %d contacts\n", len(contacts))

	// Migrate each contact
	for i, contact := range contacts {
		fmt.Printf("   [%d/%d] Migrating contact: %s:%s\n", i+1, len(contacts), contact.Channel, contact.ContactID)

		// Save to destination
		if err := dest.Contacts().Set(ctx, contact); err != nil {
			return fmt.Errorf("failed to save contact %s:%s: %w", contact.Channel, contact.ContactID, err)
		}
	}

	fmt.Printf("   ✅ Migrated %d contacts\n", len(contacts))
	return nil
}

func migrateCronJobs(ctx context.Context, source, dest storage.Storage) error {
	// List all cron jobs from source (including disabled)
	jobs, err := source.Cron().ListJobs(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to list cron jobs: %w", err)
	}

	fmt.Printf("   Found %d cron jobs\n", len(jobs))

	// Migrate each job
	for i, job := range jobs {
		fmt.Printf("   [%d/%d] Migrating cron job: %s\n", i+1, len(jobs), job.Name)

		// Save to destination
		if err := dest.Cron().AddJob(ctx, &job); err != nil {
			return fmt.Errorf("failed to save cron job %s: %w", job.Name, err)
		}
	}

	fmt.Printf("   ✅ Migrated %d cron jobs\n", len(jobs))
	return nil
}

// exportDataCommand exports data from current storage to JSON files
func exportDataCommand(outputDir string) {
	fmt.Println("📤 PicoClaw Data Export Tool")
	fmt.Println("===========================")
	fmt.Println()

	// Load configuration
	configPath := getConfigPath()
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("❌ Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Create storage
	storeCfg := storage.Config{
		Type:        cfg.Storage.Type,
		DatabaseURL: cfg.Storage.DatabaseURL,
		FilePath:    cfg.WorkspacePath(),
	}

	fmt.Printf("📁 Storage type: %s\n", cfg.Storage.Type)
	fmt.Printf("📁 Output directory: %s\n", outputDir)
	fmt.Println()

	store, err := storage.NewStorage(storeCfg)
	if err != nil {
		fmt.Printf("❌ Error creating storage: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if err := store.Connect(ctx); err != nil {
		fmt.Printf("❌ Error connecting to storage: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("❌ Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Export sessions
	fmt.Println("📦 Exporting sessions...")
	if err := exportSessions(ctx, store, outputDir); err != nil {
		fmt.Printf("❌ Error exporting sessions: %v\n", err)
		os.Exit(1)
	}

	// Export contacts
	fmt.Println("📦 Exporting contacts...")
	if err := exportContacts(ctx, store, outputDir); err != nil {
		fmt.Printf("❌ Error exporting contacts: %v\n", err)
		os.Exit(1)
	}

	// Export cron jobs
	fmt.Println("📦 Exporting cron jobs...")
	if err := exportCronJobs(ctx, store, outputDir); err != nil {
		fmt.Printf("❌ Error exporting cron jobs: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("✅ Export completed successfully to: %s\n", outputDir)
}

func exportSessions(ctx context.Context, store storage.Storage, outputDir string) error {
	sessionInfos, err := store.Sessions().List(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("   Exporting %d sessions...\n", len(sessionInfos))

	for _, info := range sessionInfos {
		session, err := store.Sessions().Get(ctx, info.Key)
		if err != nil {
			return err
		}

		// Write session to JSON file
		filename := fmt.Sprintf("%s/%s.json", outputDir, sanitizeFilename(session.Key))
		if err := writeJSON(filename, session); err != nil {
			return err
		}
	}

	fmt.Printf("   ✅ Exported %d sessions\n", len(sessionInfos))
	return nil
}

func exportContacts(ctx context.Context, store storage.Storage, outputDir string) error {
	contacts, err := store.Contacts().List(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("   Exporting %d contacts...\n", len(contacts))

	// Write all contacts to single file
	filename := fmt.Sprintf("%s/contacts.json", outputDir)
	if err := writeJSON(filename, contacts); err != nil {
		return err
	}

	fmt.Printf("   ✅ Exported %d contacts\n", len(contacts))
	return nil
}

func exportCronJobs(ctx context.Context, store storage.Storage, outputDir string) error {
	jobs, err := store.Cron().ListJobs(ctx, true)
	if err != nil {
		return err
	}

	fmt.Printf("   Exporting %d cron jobs...\n", len(jobs))

	// Write all jobs to single file
	filename := fmt.Sprintf("%s/cron_jobs.json", outputDir)
	if err := writeJSON(filename, jobs); err != nil {
		return err
	}

	fmt.Printf("   ✅ Exported %d cron jobs\n", len(jobs))
	return nil
}

// Helper functions
func writeJSON(filename string, data interface{}) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func sanitizeFilename(s string) string {
	// Replace unsafe characters for filenames
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	return s
}
