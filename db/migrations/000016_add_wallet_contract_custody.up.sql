ALTER TABLE wallets ADD COLUMN custody_type TEXT NOT NULL DEFAULT 'custodial';
ALTER TABLE wallets ADD COLUMN contract_id TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_wallets_contract_id ON wallets (contract_id) WHERE contract_id <> '';
