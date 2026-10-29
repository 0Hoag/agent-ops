CREATE TABLE alert_occurrences (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    alert_key TEXT NOT NULL,
    repository TEXT NOT NULL,
    rule TEXT NOT NULL,
    subject TEXT NOT NULL,
    message TEXT NOT NULL,
    detected_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, alert_key)
);
CREATE INDEX alert_occurrences_org_detected_idx ON alert_occurrences(organization_id, detected_at DESC);
CREATE TABLE alert_deliveries (
    id UUID PRIMARY KEY,
    occurrence_id UUID NOT NULL REFERENCES alert_occurrences(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,
    success BOOLEAN NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    delivered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (occurrence_id, channel)
);
