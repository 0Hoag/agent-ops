ALTER TABLE issues ADD COLUMN IF NOT EXISTS upstream_updated_at TIMESTAMPTZ;
ALTER TABLE pull_requests ADD COLUMN IF NOT EXISTS upstream_updated_at TIMESTAMPTZ;
ALTER TABLE pipeline_runs ADD COLUMN IF NOT EXISTS upstream_updated_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_issues_repository_upstream_updated
    ON issues(repository_id, upstream_updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_pull_requests_repository_upstream_updated
    ON pull_requests(repository_id, upstream_updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_pipeline_runs_repository_upstream_updated
    ON pipeline_runs(repository_id, upstream_updated_at DESC);
