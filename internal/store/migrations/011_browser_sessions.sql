CREATE TABLE login_challenges (
    state_hash text PRIMARY KEY,
    verifier text NOT NULL,
    nonce text NOT NULL,
    issuer text NOT NULL,
    client_id text NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX login_challenges_expiry ON login_challenges(expires_at);
CREATE TABLE browser_sessions (
    token_hash text PRIMARY KEY,
    issuer text NOT NULL,
    subject text NOT NULL,
    client_id text NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX browser_sessions_expiry ON browser_sessions(expires_at);
