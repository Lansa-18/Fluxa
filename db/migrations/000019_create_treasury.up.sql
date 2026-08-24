CREATE TABLE treasury_config (
    asset                VARCHAR(20) PRIMARY KEY,
    sweep_threshold      NUMERIC(20, 7) NOT NULL DEFAULT 0,
    min_operating_buffer NUMERIC(20, 7) NOT NULL DEFAULT 0,
    cold_storage_address TEXT NOT NULL DEFAULT '',
    auto_sweep_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO treasury_config (asset) VALUES ('XLM'), ('USDC'), ('EURC');

CREATE TABLE sweep_log (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    asset        VARCHAR(20) NOT NULL,
    amount       NUMERIC(20, 7) NOT NULL,
    destination  TEXT NOT NULL DEFAULT '',
    tx_hash      TEXT NOT NULL DEFAULT '',
    triggered_by VARCHAR(10) NOT NULL,
    swept_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sweep_log_swept_at ON sweep_log(swept_at DESC);
CREATE INDEX idx_sweep_log_asset ON sweep_log(asset);
