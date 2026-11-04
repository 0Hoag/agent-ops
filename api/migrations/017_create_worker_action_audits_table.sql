CREATE TABLE worker_action_audits (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    actor_id TEXT NOT NULL, occurred_at TIMESTAMPTZ NOT NULL, action TEXT NOT NULL,
    wave TEXT NOT NULL DEFAULT '', job_id TEXT NOT NULL DEFAULT '', outcome TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX worker_action_audits_org_time_idx ON worker_action_audits (organization_id, occurred_at DESC);
