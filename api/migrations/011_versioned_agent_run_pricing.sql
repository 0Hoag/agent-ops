CREATE TABLE agent_run_price_versions (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    model text NOT NULL,
    effective_date date NOT NULL,
    input_per_million numeric(20,10) NOT NULL CHECK (input_per_million >= 0),
    cached_input_per_million numeric(20,10) NOT NULL CHECK (cached_input_per_million >= 0),
    cache_write_per_million numeric(20,10) NOT NULL CHECK (cache_write_per_million >= 0),
    output_per_million numeric(20,10) NOT NULL CHECK (output_per_million >= 0),
    reasoning_per_million numeric(20,10) NOT NULL CHECK (reasoning_per_million >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (organization_id, model, effective_date)
);
CREATE INDEX agent_run_price_versions_lookup ON agent_run_price_versions (organization_id, model, effective_date DESC);
