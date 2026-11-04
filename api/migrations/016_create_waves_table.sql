CREATE TABLE waves (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    brief TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'todo' CHECK (state IN ('todo','in_progress','review','blocked','done')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_waves_organization_created ON waves (organization_id, created_at DESC);
