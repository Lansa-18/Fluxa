CREATE TABLE idempotency_records (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL,
    key             UUID NOT NULL,
    request_hash    TEXT NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'processing',
    response_status INT,
    response_body   JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX idx_idempotency_records_org_key ON idempotency_records(org_id, key);

ALTER TABLE transactions ADD COLUMN idempotency_key TEXT;

CREATE INDEX idx_transactions_idempotency_key ON transactions(idempotency_key) WHERE idempotency_key IS NOT NULL;
