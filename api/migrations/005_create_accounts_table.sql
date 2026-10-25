CREATE TABLE accounts (
    id uuid PRIMARY KEY,
    email text NOT NULL,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT accounts_email_nonempty CHECK (length(trim(email)) > 0),
    CONSTRAINT accounts_password_hash_nonempty CHECK (length(password_hash) > 0),
    CONSTRAINT accounts_email_unique UNIQUE (email)
);
