-- Initial schema for PicoClaw PostgreSQL storage

-- Sessions table: stores conversation sessions with message history
CREATE TABLE IF NOT EXISTS sessions (
    key VARCHAR(255) PRIMARY KEY,
    messages JSONB NOT NULL DEFAULT '[]'::jsonb,
    summary TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- Index for sorting sessions by update time
CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at DESC);

-- Index for sorting sessions by creation time
CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at DESC);

-- GIN index for JSONB queries on messages (enables efficient querying within messages)
CREATE INDEX IF NOT EXISTS idx_sessions_messages_gin ON sessions USING GIN (messages);

-- Contact instructions table: stores per-contact custom instructions
CREATE TABLE IF NOT EXISTS contact_instructions (
    channel VARCHAR(50) NOT NULL,
    contact_id VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    instructions TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    PRIMARY KEY (channel, contact_id)
);

-- Index for sorting contacts by update time
CREATE INDEX IF NOT EXISTS idx_contacts_updated_at ON contact_instructions(updated_at DESC);

-- Cron jobs table: stores scheduled automation jobs
CREATE TABLE IF NOT EXISTS cron_jobs (
    id VARCHAR(64) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    schedule JSONB NOT NULL,
    payload JSONB NOT NULL,
    state JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at_ms BIGINT NOT NULL,
    updated_at_ms BIGINT NOT NULL,
    delete_after_run BOOLEAN NOT NULL DEFAULT false
);

-- Index for enabled jobs (filtered index for performance)
CREATE INDEX IF NOT EXISTS idx_cron_enabled ON cron_jobs(enabled) WHERE enabled = true;

-- Index for next run time (used to find due jobs efficiently)
CREATE INDEX IF NOT EXISTS idx_cron_next_run ON cron_jobs((state->>'nextRunAtMs')) WHERE enabled = true;

-- Index for sorting cron jobs by update time
CREATE INDEX IF NOT EXISTS idx_cron_updated_at ON cron_jobs(updated_at_ms DESC);

-- Schema migrations table: tracks applied migrations
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INT PRIMARY KEY,
    applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Insert initial migration version
INSERT INTO schema_migrations (version) VALUES (1)
ON CONFLICT (version) DO NOTHING;
