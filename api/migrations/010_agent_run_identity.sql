ALTER TABLE agent_runs ADD COLUMN harness text;
ALTER TABLE agent_runs ADD COLUMN session_id text;
CREATE UNIQUE INDEX agent_runs_identity_idx ON agent_runs(repository_id, harness, session_id)
    WHERE harness IS NOT NULL AND session_id IS NOT NULL AND harness <> '' AND session_id <> '';
