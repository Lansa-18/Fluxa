package transfer

import (
	"context"
	"time"

	"github.com/fluxa/fluxa/internal/domain"
)

type Repository interface {
	Create(ctx context.Context, tx *domain.Transaction) error
	GetByID(ctx context.Context, id string) (*domain.Transaction, error)
	UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus, txHash string) error
	ListByWallet(ctx context.Context, walletID string, limit, offset int) ([]*domain.Transaction, error)
	UpsertByTxHash(ctx context.Context, tx *domain.Transaction) error
	ListByBatch(ctx context.Context, batchID string) ([]*domain.Transaction, error)
	CountMonthlyTransfersByTenant(ctx context.Context, tenantID string, year int, month time.Month) (int, error)
	// ExistsByTxHash reports whether a transaction with the given Stellar hash
	// has already been recorded, used to keep indexer sync idempotent.
	ExistsByTxHash(ctx context.Context, txHash string) (bool, error)
	// GetByIdempotencyKey returns the transaction previously created for this
	// org/idempotency-key pair, or domain.ErrTransactionNotFound if none exists.
	GetByIdempotencyKey(ctx context.Context, orgID, idempotencyKey string) (*domain.Transaction, error)
}
