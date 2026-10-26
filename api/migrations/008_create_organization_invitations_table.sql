CREATE TABLE organization_invitations (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    inviter_account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    role text NOT NULL,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    accepted_by_account_id uuid REFERENCES accounts(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT organization_invitations_role CHECK (role IN ('admin', 'member', 'viewer'))
);
CREATE INDEX organization_invitations_organization_idx ON organization_invitations(organization_id);
