ALTER TABLE contact_instructions
ADD COLUMN IF NOT EXISTS agent_id VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE contact_instructions
ADD COLUMN IF NOT EXISTS response_delay_seconds INTEGER NOT NULL DEFAULT 0;

ALTER TABLE contact_instructions
ADD COLUMN IF NOT EXISTS allowed_mcps JSONB NOT NULL DEFAULT '[]'::jsonb;

INSERT INTO schema_migrations (version) VALUES (3)
ON CONFLICT (version) DO NOTHING;
