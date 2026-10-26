ALTER TABLE repositories ADD COLUMN organization_id uuid REFERENCES organizations(id) ON DELETE RESTRICT;
CREATE INDEX repositories_organization_idx ON repositories(organization_id);
