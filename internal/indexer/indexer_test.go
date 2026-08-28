package indexer

import (
	"context"
	"fmt"
	"testing"

	"github.com/fluxa/fluxa/internal/domain"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/protocols/horizon/operations"
	"github.com/stellar/go/txnbuild"
)

// --- mock wallet repo ---

type mockWalletRepo struct {
	wallets  map[string]*domain.Wallet
	cursors  map[string]string
	listErr  error
	updateErr error
}

func newMockWalletRepo() *mockWalletRepo {
	return &mockWalletRepo{
		wallets: make(map[string]*domain.Wallet),
		cursors: make(map[string]string),
	}
}

func (m *mockWalletRepo) Create(_ context.Context, w *domain.Wallet) error {
	m.wallets[w.ID] = w
	return nil
}

func (m *mockWalletRepo) GetByID(_ context.Context, id string) (*domain.Wallet, error) {
	w, ok := m.wallets[id]
	if !ok {
		return nil, domain.ErrWalletNotFound
	}
	return w, nil
}

func (m *mockWalletRepo) GetByPublicKey(_ context.Context, pubKey string) (*domain.Wallet, error) {
	for _, w := range m.wallets {
		if w.PublicKey == pubKey {
			return w, nil
		}
	}
	return nil, domain.ErrWalletNotFound
}

func (m *mockWalletRepo) List(_ context.Context, _, _ int) ([]*domain.Wallet, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var out []*domain.Wallet
	for _, w := range m.wallets {
		out = append(out, w)
	}
	return out, nil
}

func (m *mockWalletRepo) UpdateSyncCursor(_ context.Context, walletID, cursor string) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.cursors[walletID] = cursor
	if w, ok := m.wallets[walletID]; ok {
		w.SyncCursor = cursor
	}
	return nil
}

// --- mock tx repo ---

type mockTxRepo struct {
	transactions map[string]*domain.Transaction
	upsertErr    error
}

func newMockTxRepo() *mockTxRepo {
	return &mockTxRepo{transactions: make(map[string]*domain.Transaction)}
}

func (m *mockTxRepo) Create(_ context.Context, tx *domain.Transaction) error {
	m.transactions[tx.ID] = tx
	return nil
}

func (m *mockTxRepo) GetByID(_ context.Context, id string) (*domain.Transaction, error) {
	tx, ok := m.transactions[id]
	if !ok {
		return nil, domain.ErrTransactionNotFound
	}
	return tx, nil
}

func (m *mockTxRepo) UpdateStatus(_ context.Context, id string, status domain.TransactionStatus, txHash string) error {
	if tx, ok := m.transactions[id]; ok {
		tx.Status = status
		tx.TxHash = txHash
	}
	return nil
}

func (m *mockTxRepo) ListByWallet(_ context.Context, walletID string, _, _ int) ([]*domain.Transaction, error) {
	var out []*domain.Transaction
	for _, tx := range m.transactions {
		if tx.ToWallet == walletID || tx.FromWallet == walletID {
			out = append(out, tx)
		}
	}
	return out, nil
}

func (m *mockTxRepo) UpsertByTxHash(_ context.Context, tx *domain.Transaction) error {
	if m.upsertErr != nil {
		return m.upsertErr
	}
	// Idempotent: skip if tx_hash already exists
	for _, existing := range m.transactions {
		if existing.TxHash == tx.TxHash {
			return nil
		}
	}
	m.transactions[tx.ID] = tx
	return nil
}

// --- mock stellar client ---

type mockStellarClient struct {
	payments []horizon.Payment
	payErr   error
}

func (m *mockStellarClient) LoadAccount(_ string) (horizon.Account, error) {
	return horizon.Account{}, nil
}

func (m *mockStellarClient) SubmitTransaction(_ *txnbuild.Transaction) (horizon.Transaction, error) {
	return horizon.Transaction{}, nil
}

func (m *mockStellarClient) FindPathsStrict(_, _, _, _ string) ([]horizon.Path, error) {
	return nil, nil
}

func (m *mockStellarClient) TransactionDetail(_ string) (horizon.Transaction, error) {
	return horizon.Transaction{}, nil
}

func (m *mockStellarClient) OperationsForTransaction(_ string) ([]operations.Operation, error) {
	return nil, nil
}

func (m *mockStellarClient) PaymentsForAccount(_, cursor string, _ int) ([]horizon.Payment, error) {
	if m.payErr != nil {
		return nil, m.payErr
	}
	return m.payments, nil
}

// --- Tests ---

func TestSyncWallet_SetsTenantID(t *testing.T) {
	tenantID := "tenant-abc"
	w := &domain.Wallet{
		ID:        "w-1",
		PublicKey: "GBTEST...",
		TenantID:  &tenantID,
	}

	walletRepo := newMockWalletRepo()
	walletRepo.wallets[w.ID] = w

	txRepo := newMockTxRepo()
	stellar := &mockStellarClient{
		payments: []horizon.Payment{
			{ID: "p1", TransactionHash: "tx-hash-1", AssetType: "credit_alphanum4", AssetCode: "USDC", Amount: "10.5"},
		},
	}

	idx := New(walletRepo, txRepo, stellar)
	if err := idx.SyncWallet(context.Background(), w); err != nil {
		t.Fatalf("SyncWallet() error: %v", err)
	}

	if len(txRepo.transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(txRepo.transactions))
	}

	for _, tx := range txRepo.transactions {
		if tx.TenantID == nil || *tx.TenantID != "tenant-abc" {
			t.Fatalf("expected TenantID=tenant-abc, got %v", tx.TenantID)
		}
		if tx.ToWallet != "w-1" {
			t.Fatalf("expected ToWallet=w-1, got %s", tx.ToWallet)
		}
	}
}

func TestSyncWallet_CursorAdvances(t *testing.T) {
	w := &domain.Wallet{ID: "w-2", PublicKey: "GBTEST2..."}

	walletRepo := newMockWalletRepo()
	walletRepo.wallets[w.ID] = w

	txRepo := newMockTxRepo()
	stellar := &mockStellarClient{
		payments: []horizon.Payment{
			{ID: "p1", TransactionHash: "hash-1", AssetType: "native", Amount: "5"},
			{ID: "p2", TransactionHash: "hash-2", AssetType: "credit_alphanum4", AssetCode: "EURC", Amount: "20"},
		},
	}

	idx := New(walletRepo, txRepo, stellar)
	if err := idx.SyncWallet(context.Background(), w); err != nil {
		t.Fatalf("SyncWallet() error: %v", err)
	}

	if w.SyncCursor != "p2" {
		t.Fatalf("expected sync_cursor=p2, got %s", w.SyncCursor)
	}
	if walletRepo.cursors["w-2"] != "p2" {
		t.Fatalf("expected cursor update to p2, got %s", walletRepo.cursors["w-2"])
	}
}

func TestSyncWallet_NoPaymentsNoCursorUpdate(t *testing.T) {
	w := &domain.Wallet{ID: "w-3", PublicKey: "GBTEST3...", SyncCursor: "old-cursor"}

	walletRepo := newMockWalletRepo()
	walletRepo.wallets[w.ID] = w

	txRepo := newMockTxRepo()
	stellar := &mockStellarClient{payments: []horizon.Payment{}}

	idx := New(walletRepo, txRepo, stellar)
	if err := idx.SyncWallet(context.Background(), w); err != nil {
		t.Fatalf("SyncWallet() error: %v", err)
	}

	if w.SyncCursor != "old-cursor" {
		t.Fatalf("expected cursor to remain old-cursor, got %s", w.SyncCursor)
	}
}

func TestSyncWallet_IdempotentUnderDuplicateHashes(t *testing.T) {
	w := &domain.Wallet{ID: "w-4", PublicKey: "GBTEST4..."}

	walletRepo := newMockWalletRepo()
	walletRepo.wallets[w.ID] = w

	txRepo := newMockTxRepo()
	stellar := &mockStellarClient{
		payments: []horizon.Payment{
			{ID: "p1", TransactionHash: "dup-hash", AssetType: "native", Amount: "1"},
		},
	}

	idx := New(walletRepo, txRepo, stellar)

	// First sync
	if err := idx.SyncWallet(context.Background(), w); err != nil {
		t.Fatalf("first SyncWallet() error: %v", err)
	}
	countAfterFirst := len(txRepo.transactions)

	// Second sync with same payment — should not create duplicate
	if err := idx.SyncWallet(context.Background(), w); err != nil {
		t.Fatalf("second SyncWallet() error: %v", err)
	}
	countAfterSecond := len(txRepo.transactions)

	if countAfterFirst != countAfterSecond {
		t.Fatalf("expected idempotent insert: first=%d, second=%d", countAfterFirst, countAfterSecond)
	}
}

func TestSyncWallet_404IsNoOp(t *testing.T) {
	w := &domain.Wallet{ID: "w-5", PublicKey: "GBTEST5..."}

	walletRepo := newMockWalletRepo()
	walletRepo.wallets[w.ID] = w

	txRepo := newMockTxRepo()
	stellar := &mockStellarClient{
		payErr: fmt.Errorf("horizon error 404"),
	}

	idx := New(walletRepo, txRepo, stellar)
	if err := idx.SyncWallet(context.Background(), w); err != nil {
		t.Fatalf("SyncWallet() should return nil for 404, got: %v", err)
	}
}

func TestExtractAssetCode(t *testing.T) {
	tests := []struct {
		name     string
		payment  horizon.Payment
		expected string
	}{
		{"USDC", horizon.Payment{AssetCode: "USDC"}, "USDC"},
		{"native", horizon.Payment{AssetType: "native"}, "XLM"},
		{"fallback", horizon.Payment{AssetType: "credit_alphanum12"}, "credit_alphanum12"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractAssetCode(tt.payment)
			if got != tt.expected {
				t.Fatalf("extractAssetCode() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNewInboundTransaction_TenantID(t *testing.T) {
	tenantID := "tenant-xyz"
	tx := newInboundTransaction("w-1", "hash-1", "USDC", "100", &tenantID)

	if tx.TenantID == nil || *tx.TenantID != "tenant-xyz" {
		t.Fatalf("expected TenantID=tenant-xyz, got %v", tx.TenantID)
	}
	if tx.Asset != "USDC" {
		t.Fatalf("expected Asset=USDC, got %s", tx.Asset)
	}
	if tx.Status != domain.StatusConfirmed {
		t.Fatalf("expected status=confirmed, got %s", tx.Status)
	}
}

func TestNewInboundTransaction_NilTenantID(t *testing.T) {
	tx := newInboundTransaction("w-2", "hash-2", "XLM", "50", nil)

	if tx.TenantID != nil {
		t.Fatalf("expected nil TenantID, got %v", tx.TenantID)
	}
}
