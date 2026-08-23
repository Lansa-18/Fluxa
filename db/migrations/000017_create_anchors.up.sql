CREATE TABLE anchors (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    home_domain        TEXT NOT NULL UNIQUE,
    transfer_server    TEXT NOT NULL DEFAULT '',
    transfer_server_sep24 TEXT NOT NULL DEFAULT '',
    web_auth_endpoint  TEXT NOT NULL DEFAULT '',
    sep10_signing_key  TEXT NOT NULL DEFAULT '',
    network_passphrase TEXT NOT NULL DEFAULT '',
    supported_assets   JSONB NOT NULL DEFAULT '[]',
    sep_versions       JSONB NOT NULL DEFAULT '[]',
    registered_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_anchors_home_domain ON anchors(home_domain);

CREATE TABLE anchor_transactions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID REFERENCES users(id),
    wallet_id      UUID NOT NULL REFERENCES wallets(id),
    anchor_id      UUID NOT NULL REFERENCES anchors(id),
    external_tx_id TEXT NOT NULL DEFAULT '',
    asset          VARCHAR(20) NOT NULL,
    amount         TEXT NOT NULL DEFAULT '',
    type           VARCHAR(20) NOT NULL,
    status         VARCHAR(50) NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at   TIMESTAMPTZ
);

CREATE INDEX idx_anchor_transactions_wallet_id ON anchor_transactions(wallet_id);
CREATE INDEX idx_anchor_transactions_anchor_id ON anchor_transactions(anchor_id);
CREATE INDEX idx_anchor_transactions_external_tx_id ON anchor_transactions(anchor_id, external_tx_id);
