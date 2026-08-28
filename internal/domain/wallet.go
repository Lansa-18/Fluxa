package domain

import "time"

type Wallet struct {
	ID              string
	PublicKey       string
	EncryptedSecret string
	TenantID        *string
	SyncCursor      string
	CreatedAt       time.Time
}
