ALTER TABLE agent_runs ADD COLUMN organization_id uuid REFERENCES organizations(id);
UPDATE agent_runs ar SET organization_id = repo.organization_id FROM repositories repo WHERE repo.id = ar.repository_id;
ALTER TABLE agent_runs ALTER COLUMN organization_id SET NOT NULL;
ALTER TABLE agent_runs ALTER COLUMN repository_id DROP NOT NULL;
DROP INDEX IF EXISTS agent_runs_identity_idx;
CREATE UNIQUE INDEX agent_runs_repo_identity_idx ON agent_runs(repository_id, harness, session_id)
    WHERE repository_id IS NOT NULL AND harness IS NOT NULL AND session_id IS NOT NULL AND harness <> '' AND session_id <> '';
CREATE UNIQUE INDEX agent_runs_local_identity_idx ON agent_runs(organization_id, harness, session_id)
    WHERE repository_id IS NULL AND harness IS NOT NULL AND session_id IS NOT NULL AND harness <> '' AND session_id <> '';
CREATE INDEX agent_runs_organization_id_idx ON agent_runs(organization_id);
