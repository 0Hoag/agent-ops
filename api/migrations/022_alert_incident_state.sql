ALTER TABLE alert_occurrences DROP CONSTRAINT alert_occurrences_organization_id_alert_key_key;
CREATE TABLE alert_incident_states (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    alert_key TEXT NOT NULL,
    active BOOLEAN NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, alert_key)
);
