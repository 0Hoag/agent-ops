CREATE TABLE organizations (
    id uuid PRIMARY KEY,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT organizations_name_nonempty CHECK (length(trim(name)) > 0)
);

CREATE TABLE organization_memberships (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    account_id uuid NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT organization_memberships_role CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    CONSTRAINT organization_memberships_unique UNIQUE (organization_id, account_id)
);
CREATE INDEX organization_memberships_account_idx ON organization_memberships(account_id);
CREATE INDEX organization_memberships_organization_idx ON organization_memberships(organization_id);
