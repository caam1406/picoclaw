-- Additional indexes for performance optimization

-- Full-text search index for contact instructions (if needed in the future)
CREATE INDEX IF NOT EXISTS idx_contacts_instructions_fts
ON contact_instructions USING GIN (to_tsvector('english', instructions));

-- Index for session message count (helps with queries that filter by message count)
CREATE INDEX IF NOT EXISTS idx_sessions_message_count
ON sessions((jsonb_array_length(messages)));

-- Insert migration version
INSERT INTO schema_migrations (version) VALUES (2)
ON CONFLICT (version) DO NOTHING;
