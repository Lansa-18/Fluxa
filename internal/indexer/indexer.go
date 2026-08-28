package indexer

import (
	"context"
	"fmt"
	"strings"

	"github.com/fluxa/fluxa/internal/domain"
	"github.com/fluxa/fluxa/internal/stellar"
	"github.com/fluxa/fluxa/internal/transfer"
	"github.com/fluxa/fluxa/internal/wallet"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/stellar/go/protocols/horizon"
	"time"
)

const syncPageSize = 100

type Indexer struct {
	walletRepo wallet.Repository
	txRepo     transfer.Repository
	stellar    stellar.Client
}

func New(walletRepo wallet.Repository, txRepo transfer.Repository, stellarClient stellar.Client) *Indexer {
	return &Indexer{
		walletRepo: walletRepo,
		txRepo:     txRepo,
		stellar:    stellarClient,
	}
}

// SyncAll iterates over all wallets and syncs their recent payments from Horizon.
func (idx *Indexer) SyncAll(ctx context.Context, limit, offset int) error {
	wallets, err := idx.walletRepo.List(ctx, limit, offset)
	if err != nil {
		return fmt.Errorf("list wallets: %w", err)
	}

	for _, w := range wallets {
		if err := idx.SyncWallet(ctx, w); err != nil {
			log.Error().Err(err).Str("wallet_id", w.ID).Msg("failed to sync wallet")
		}
	}
	return nil
}

// SyncWallet syncs Horizon payment history for a single wallet into the local DB.
// It uses the wallet's sync_cursor to fetch only new payments since the last sync,
// and sets TenantID from the indexed wallet on every created transaction.
func (idx *Indexer) SyncWallet(ctx context.Context, w *domain.Wallet) error {
	payments, err := idx.stellar.PaymentsForAccount(w.PublicKey, w.SyncCursor, syncPageSize)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil // account not yet funded — nothing to sync
		}
		return fmt.Errorf("load payments: %w", err)
	}

	var lastCursor string
	for _, p := range payments {
		tx := newInboundTransaction(w.ID, p.TransactionHash, extractAssetCode(p), p.Amount, w.TenantID)
		if err := idx.txRepo.UpsertByTxHash(ctx, tx); err != nil {
			log.Error().Err(err).Str("wallet_id", w.ID).Str("tx_hash", p.TransactionHash).Msg("failed to upsert inbound transaction")
			continue
		}
		lastCursor = p.ID
	}

	if lastCursor != "" {
		if err := idx.walletRepo.UpdateSyncCursor(ctx, w.ID, lastCursor); err != nil {
			log.Error().Err(err).Str("wallet_id", w.ID).Msg("failed to update sync cursor")
		}
	}

	return nil
}

// newInboundTransaction creates a confirmed inbound transaction with the wallet's tenant identity.
func newInboundTransaction(walletID, txHash, asset, amount string, tenantID *string) *domain.Transaction {
	amt, _ := decimal.NewFromString(amount)
	return &domain.Transaction{
		ID:        uuid.New().String(),
		TxHash:    txHash,
		Type:      domain.TypeTransfer,
		Status:    domain.StatusConfirmed,
		ToWallet:  walletID,
		Asset:     asset,
		Amount:    amt,
		TenantID:  tenantID,
		CreatedAt: time.Now().UTC(),
	}
}

// extractAssetCode returns a human-readable asset code from a Horizon payment.
func extractAssetCode(p horizon.Payment) string {
	if p.AssetCode != "" {
		return p.AssetCode
	}
	if p.AssetType == "native" {
		return "XLM"
	}
	return p.AssetType
}
