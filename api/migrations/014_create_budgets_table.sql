CREATE TABLE budgets (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    repository_id uuid REFERENCES repositories(id) ON DELETE CASCADE,
    wave text NOT NULL DEFAULT '',
    token_limit bigint,
    usd_limit numeric(20,6),
    starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT NOW(),
    updated_at timestamptz NOT NULL DEFAULT NOW(),
    CHECK (repository_id IS NULL OR wave <> '' OR token_limit IS NOT NULL OR usd_limit IS NOT NULL),
    CHECK (token_limit IS NULL OR token_limit > 0),
    CHECK (usd_limit IS NULL OR usd_limit > 0),
    CHECK (ends_at > starts_at),
    CHECK (token_limit IS NOT NULL OR usd_limit IS NOT NULL)
);
CREATE INDEX budgets_organization_idx ON budgets(organization_id);
CREATE INDEX budgets_repository_idx ON budgets(repository_id);
