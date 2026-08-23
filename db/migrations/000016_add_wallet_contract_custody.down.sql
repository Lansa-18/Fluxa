DROP INDEX IF EXISTS idx_wallets_contract_id;

ALTER TABLE wallets DROP COLUMN contract_id;
ALTER TABLE wallets DROP COLUMN custody_type;
