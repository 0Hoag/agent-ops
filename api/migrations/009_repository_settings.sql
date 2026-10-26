ALTER TABLE repositories ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations(id) ON DELETE RESTRICT;
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS webhook_secret_ciphertext TEXT NOT NULL DEFAULT '';
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS backfill_token_ciphertext TEXT NOT NULL DEFAULT '';
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS git_lab_base_url TEXT NOT NULL DEFAULT '';
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS status_mappings JSONB NOT NULL DEFAULT '{}'::jsonb;
CREATE INDEX IF NOT EXISTS repositories_organization_idx ON repositories(organization_id);
