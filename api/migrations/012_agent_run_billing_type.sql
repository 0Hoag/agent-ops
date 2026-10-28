ALTER TABLE agent_runs ADD COLUMN billing_type text NOT NULL DEFAULT 'api';
ALTER TABLE agent_runs ADD CONSTRAINT agent_runs_billing_type_check CHECK (billing_type IN ('api', 'subscription'));
