CREATE TABLE IF NOT EXISTS entities (
    uuid UUID PRIMARY KEY,
    parent_uuid UUID NULL REFERENCES entities(uuid), -- Handles Series->Season hierarchy
    entity_type VARCHAR(50) NOT NULL,                -- Matches config (e.g., "series", "episode")
    status VARCHAR(20) NOT NULL,                     -- WANTED, AVAILABLE, DOWNLOADED, MISSING
    monitored BOOLEAN DEFAULT TRUE,
    last_refreshed_at TIMESTAMP,                     -- For TTL Reconciliation loop
    quality_profile_id INT,
    local_path TEXT,                                 -- Location on Media PVC
    metadata JSONB                                   -- Full blob from Metadata Provider (TMDB/TVDB)
);

CREATE TABLE IF NOT EXISTS idx_identifiers (
    entity_uuid UUID REFERENCES entities(uuid) ON DELETE CASCADE,
    key VARCHAR(50) NOT NULL,   -- e.g., "imdb_id", "isbn", "slug"
    value VARCHAR(255) NOT NULL,-- e.g., "tt12345", "978-3-16-148410-0"
    PRIMARY KEY (entity_uuid, key)
);

CREATE INDEX IF NOT EXISTS idx_lookup ON idx_identifiers(key, value);