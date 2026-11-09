ALTER TABLE repositories ADD COLUMN IF NOT EXISTS last_synced_at timestamptz;
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS last_sync_status text NOT NULL DEFAULT '';
ALTER TABLE repositories ADD COLUMN IF NOT EXISTS last_sync_error text NOT NULL DEFAULT '';
