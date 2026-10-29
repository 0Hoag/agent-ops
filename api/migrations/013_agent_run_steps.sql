CREATE TABLE agent_run_steps (
    id uuid PRIMARY KEY, run_id uuid NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    sequence integer NOT NULL, started_at timestamptz, ended_at timestamptz,
    input_tokens bigint NOT NULL DEFAULT 0, cached_input_tokens bigint NOT NULL DEFAULT 0,
    cache_write_tokens bigint NOT NULL DEFAULT 0, output_tokens bigint NOT NULL DEFAULT 0,
    reasoning_tokens bigint NOT NULL DEFAULT 0, tool_name text NOT NULL DEFAULT '',
    cache_hit boolean NOT NULL DEFAULT false, cumulative_cost_usd double precision,
    UNIQUE(run_id, sequence)
);
CREATE INDEX agent_run_steps_run_idx ON agent_run_steps(run_id, sequence);
