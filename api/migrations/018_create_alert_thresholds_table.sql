CREATE TABLE alert_thresholds (
    organization_id uuid PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    min_cache_hit numeric(5,4) NOT NULL CHECK (min_cache_hit >= 0 AND min_cache_hit <= 1),
    silent_seconds bigint NOT NULL CHECK (silent_seconds > 0),
    stale_seconds bigint NOT NULL CHECK (stale_seconds > 0),
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW()
);
