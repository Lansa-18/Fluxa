package domain

import "time"

// CustodyType distinguishes wallets whose secret key Fluxa holds from
// contract wallets, where the key never reaches Fluxa at all.
type CustodyType string

const (
	CustodyCustodial CustodyType = "custodial"
	CustodyContract  CustodyType = "contract"
)

type Wallet struct {
	ID              string
	PublicKey       string
	EncryptedSecret string
	TenantID        *string
	CreatedAt       time.Time
	// SyncCursor is the Horizon paging token of the last payment operation
	// processed for this wallet, used to resume incremental sync.
	SyncCursor string
	// CustodyType records which wallet adapter owns this wallet.
	CustodyType CustodyType
	// ContractID is the deployed Soroban contract address for contract
	// wallets, and empty for custodial wallets.
	ContractID string
}

type BalanceRecord struct {
	WalletID  string
	AssetCode string
	Issuer    string
	Balance   string
	UpdatedAt time.Time
}
