package treasury

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// Config holds the per-asset sweep policy stored in treasury_config.
type Config struct {
	Asset              string
	SweepThreshold     decimal.Decimal
	MinOperatingBuffer decimal.Decimal
	ColdStorageAddress string
	AutoSweepEnabled   bool
	UpdatedAt          time.Time
}

// SweepLog is one row of the sweep audit trail, written on every sweep
// attempt — including "zero sweeps" where nothing was moved.
type SweepLog struct {
	ID          string
	Asset       string
	Amount      decimal.Decimal
	Destination string
	TxHash      string
	TriggeredBy string // "auto" | "manual"
	SweptAt     time.Time
}

const (
	TriggeredByAuto   = "auto"
	TriggeredByManual = "manual"
)

// Repository persists treasury sweep configuration and the sweep audit log.
type Repository interface {
	GetConfig(ctx context.Context, asset string) (*Config, error)
	ListConfig(ctx context.Context) ([]*Config, error)
	UpdateConfig(ctx context.Context, cfg *Config) error
	// ListWalletPublicKeys returns the Stellar public key of every wallet
	// Fluxa custodies, across all tenants — treasury reserve accounting is a
	// platform-wide concern, not scoped to a single org.
	ListWalletPublicKeys(ctx context.Context) ([]string, error)
	RecordSweep(ctx context.Context, log *SweepLog) error
	ListSweeps(ctx context.Context, limit, offset int) ([]*SweepLog, error)
}
