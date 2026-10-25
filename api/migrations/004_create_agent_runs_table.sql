CREATE TABLE agent_runs (
    id uuid PRIMARY KEY,
    repository_id uuid NOT NULL REFERENCES repositories(id),
    wave text NOT NULL,
    worker text NOT NULL,
    model text NOT NULL,
    issue_numbers jsonb NOT NULL DEFAULT '[]',
    pr_number bigint,
    input_tokens bigint NOT NULL CHECK (input_tokens >= 0),
    cached_input_tokens bigint NOT NULL CHECK (cached_input_tokens >= 0),
    cache_write_tokens bigint NOT NULL CHECK (cache_write_tokens >= 0),
    output_tokens bigint NOT NULL CHECK (output_tokens >= 0),
    reasoning_tokens bigint NOT NULL CHECK (reasoning_tokens >= 0),
    cost_usd numeric(20,10),
    started_at timestamptz NOT NULL,
    ended_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ended_at >= started_at),
    CHECK (issue_numbers IS NOT NULL AND jsonb_typeof(issue_numbers) = 'array')
);
CREATE INDEX agent_runs_repository_id_idx ON agent_runs(repository_id);
CREATE INDEX agent_runs_wave_idx ON agent_runs(wave);
CREATE INDEX agent_runs_worker_idx ON agent_runs(worker);
CREATE INDEX agent_runs_issue_numbers_idx ON agent_runs USING gin(issue_numbers);
