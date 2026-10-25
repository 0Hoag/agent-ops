CREATE TABLE IF NOT EXISTS repositories (
    id UUID PRIMARY KEY,
    provider TEXT NOT NULL,
    full_name TEXT NOT NULL,
    webhook_secret_env_ref TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, full_name)
);

CREATE TABLE IF NOT EXISTS issues (
    id UUID PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    number BIGINT NOT NULL,
    title TEXT NOT NULL,
    state TEXT NOT NULL,
    status TEXT NOT NULL,
    labels JSONB NOT NULL DEFAULT '[]'::jsonb,
    assignee TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (repository_id, number)
);

CREATE TABLE IF NOT EXISTS pull_requests (
    id UUID PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    number BIGINT NOT NULL,
    title TEXT NOT NULL,
    state TEXT NOT NULL,
    head_sha TEXT NOT NULL,
    branch TEXT NOT NULL,
    closes JSONB NOT NULL DEFAULT '[]'::jsonb,
    merged_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (repository_id, number)
);

CREATE TABLE IF NOT EXISTS pipeline_runs (
    id UUID PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    external_id TEXT NOT NULL,
    pr_number BIGINT,
    sha TEXT NOT NULL,
    status TEXT NOT NULL,
    conclusion TEXT NOT NULL,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (repository_id, external_id)
);

CREATE TABLE IF NOT EXISTS webhook_events (
    id UUID PRIMARY KEY,
    repository_id UUID NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    event_type TEXT NOT NULL,
    delivery_id TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (repository_id, provider, delivery_id)
);

CREATE INDEX IF NOT EXISTS idx_issues_repository_updated ON issues(repository_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_pull_requests_repository_updated ON pull_requests(repository_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_repository_sha ON pipeline_runs(repository_id, sha);
CREATE INDEX IF NOT EXISTS idx_webhook_events_repository_received ON webhook_events(repository_id, received_at DESC);
