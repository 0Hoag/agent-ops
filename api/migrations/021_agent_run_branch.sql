ALTER TABLE agent_runs ADD COLUMN branch text NOT NULL DEFAULT '';
CREATE INDEX agent_runs_branch_idx ON agent_runs(repository_id, branch);
