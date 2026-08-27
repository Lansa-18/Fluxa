package fiat

import (
	"context"
	"errors"
	"testing"

	"github.com/fluxa/fluxa/internal/domain"
	"github.com/fluxa/fluxa/internal/fx"
	"github.com/fluxa/fluxa/internal/stellar"
	"github.com/fluxa/fluxa/internal/transfer"
	"github.com/shopspring/decimal"
)

// mockRepository implements fiat.Repository for testing
type mockRepository struct {
	withdrawals map[string]*domain.FiatWithdrawal
	createErr   error
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		withdrawals: make(map[string]*domain.FiatWithdrawal),
	}
}

func (m *mockRepository) CreateDeposit(ctx context.Context, d *domain.FiatDeposit) error {
	return nil
}

func (m *mockRepository) UpdateDepositStatus(ctx context.Context, id, status string) error {
	return nil
}

func (m *mockRepository) GetDepositByReference(ctx context.Context, ref string) (*domain.FiatDeposit, error) {
	return nil, nil
}

func (m *mockRepository) CreateWithdrawal(ctx context.Context, w *domain.FiatWithdrawal) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.withdrawals[w.ID] = w
	return nil
}

func (m *mockRepository) UpdateWithdrawalStatus(ctx context.Context, id, status string) error {
	if w, ok := m.withdrawals[id]; ok {
		w.Status = status
		return nil
	}
	return errors.New("withdrawal not found")
}

func (m *mockRepository) GetWithdrawalByReference(ctx context.Context, ref string) (*domain.FiatWithdrawal, error) {
	for _, w := range m.withdrawals {
		if w.ProviderReference == ref {
			return w, nil
		}
	}
	return nil, errors.New("not found")
}

// mockRail implements fiat.Rail for testing
type mockRail struct {
	withdrawResp *WithdrawResponse
	withdrawErr  error
}

func (m *mockRail) Deposit(ctx context.Context, req DepositRequest) (*DepositResponse, error) {
	return nil, nil
}

func (m *mockRail) Withdraw(ctx context.Context, req WithdrawRequest) (*WithdrawResponse, error) {
	if m.withdrawErr != nil {
		return nil, m.withdrawErr
	}
	return m.withdrawResp, nil
}

func (m *mockRail) HandleWebhook(ctx context.Context, payload []byte, signature string) (*RailEvent, error) {
	return nil, nil
}

// mockFXService implements fx.Service for testing
type mockFXService struct {
	quote *fx.Quote
	err   error
}

func (m *mockFXService) GetQuote(ctx context.Context, fromAsset, toAsset, amount string) (*fx.Quote, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.quote, nil
}

func (m *mockFXService) ExecuteConversion(ctx context.Context, walletID, quoteID string) (*domain.Conversion, error) {
	return nil, nil
}

func (m *mockFXService) GetRates(ctx context.Context, from, to string) (*fx.RateResponse, error) {
	return nil, nil
}

// mockTransferService implements transfer.Service for testing
type mockTransferService struct {
	transferErr error
	transfers   []struct {
		fromID, toID, asset string
		amount              decimal.Decimal
	}
}

func (m *mockTransferService) InitiateTransfer(ctx context.Context, fromID, toID, asset string, amount decimal.Decimal) (*domain.Transaction, error) {
	if m.transferErr != nil {
		return nil, m.transferErr
	}
	m.transfers = append(m.transfers, struct {
		fromID, toID, asset string
		amount              decimal.Decimal
	}{fromID, toID, asset, amount})
	return &domain.Transaction{ID: "tx-123"}, nil
}

func (m *mockTransferService) InitiateTransferIdempotent(ctx context.Context, fromID, toID, asset string, amount decimal.Decimal, idempotencyKey string) (*domain.Transaction, error) {
	return nil, nil
}

func (m *mockTransferService) InitiateBatchTransfer(ctx context.Context, fromID, toID, asset string, amount decimal.Decimal, batchID, reference string) (*domain.Transaction, error) {
	return nil, nil
}

func (m *mockTransferService) GetTransaction(ctx context.Context, id string) (*domain.Transaction, error) {
	return nil, nil
}

func (m *mockTransferService) ListTransactions(ctx context.Context, walletID string, limit, offset int) ([]*domain.Transaction, error) {
	return nil, nil
}

func (m *mockTransferService) WithStellarClient(stellarClient stellar.Client) transfer.Service {
	return m
}

func TestInitiateWithdrawal_Success(t *testing.T) {
	repo := newMockRepository()
	rail := &mockRail{
		withdrawResp: &WithdrawResponse{Reference: "REF-123", Status: "completed"},
	}
	fxSvc := &mockFXService{
		quote: &fx.Quote{
			Rate: decimal.NewFromInt(1600),
		},
	}
	transferSvc := &mockTransferService{}

	svc := NewService(repo, rail, fxSvc, transferSvc, "platform-wallet-123", "flutterwave")

	req := WithdrawRequest{
		WalletID:      "wallet-123",
		Reference:     "REF-123",
		FiatAmount:    decimal.NewFromInt(16000), // 16000 NGN
		FiatCurrency:  "NGN",
		AccountBank:   "044",
		AccountNumber: "0123456789",
	}

	resp, err := svc.InitiateWithdrawal(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Reference != "REF-123" {
		t.Errorf("expected reference REF-123, got %s", resp.Reference)
	}

	// 16000 NGN / 1600 NGN/USDC = 10 USDC
	expectedUSDCAmount := decimal.NewFromInt(10)

	// Verify withdrawal record was created with correct USDC amount
	var createdWithdrawal *domain.FiatWithdrawal
	for _, w := range repo.withdrawals {
		createdWithdrawal = w
		break
	}

	if createdWithdrawal == nil {
		t.Fatal("withdrawal record was not created")
	}

	if !createdWithdrawal.USDCAmount.Equal(expectedUSDCAmount) {
		t.Errorf("expected USDC amount %s, got %s", expectedUSDCAmount, createdWithdrawal.USDCAmount)
	}

	// Verify transfer was initiated with correct details
	if len(transferSvc.transfers) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(transferSvc.transfers))
	}

	tr := transferSvc.transfers[0]
	if tr.fromID != "wallet-123" || tr.toID != "platform-wallet-123" || tr.asset != "USDC" || !tr.amount.Equal(expectedUSDCAmount) {
		t.Errorf("unexpected transfer details: %+v", tr)
	}
}

func TestInitiateWithdrawal_FXServiceFailure(t *testing.T) {
	repo := newMockRepository()
	rail := &mockRail{}
	fxSvc := &mockFXService{
		err: errors.New("FX provider down"),
	}
	transferSvc := &mockTransferService{}

	svc := NewService(repo, rail, fxSvc, transferSvc, "platform-wallet-123", "flutterwave")

	req := WithdrawRequest{
		WalletID:     "wallet-123",
		Reference:    "REF-123",
		FiatAmount:   decimal.NewFromInt(16000),
		FiatCurrency: "NGN",
	}

	_, err := svc.InitiateWithdrawal(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(repo.withdrawals) != 0 {
		t.Errorf("expected 0 withdrawals in repo, got %d", len(repo.withdrawals))
	}

	if len(transferSvc.transfers) != 0 {
		t.Errorf("expected 0 transfers, got %d", len(transferSvc.transfers))
	}
}

func TestInitiateWithdrawal_FXZeroRate(t *testing.T) {
	repo := newMockRepository()
	rail := &mockRail{}
	fxSvc := &mockFXService{
		quote: &fx.Quote{
			Rate: decimal.Zero,
		},
	}
	transferSvc := &mockTransferService{}

	svc := NewService(repo, rail, fxSvc, transferSvc, "platform-wallet-123", "flutterwave")

	req := WithdrawRequest{
		WalletID:     "wallet-123",
		Reference:    "REF-123",
		FiatAmount:   decimal.NewFromInt(16000),
		FiatCurrency: "NGN",
	}

	_, err := svc.InitiateWithdrawal(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(repo.withdrawals) != 0 {
		t.Errorf("expected 0 withdrawals in repo, got %d", len(repo.withdrawals))
	}
}

func TestInitiateWithdrawal_TransferFailure(t *testing.T) {
	repo := newMockRepository()
	rail := &mockRail{}
	fxSvc := &mockFXService{
		quote: &fx.Quote{
			Rate: decimal.NewFromInt(1600),
		},
	}
	transferSvc := &mockTransferService{
		transferErr: errors.New("insufficient balance"),
	}

	svc := NewService(repo, rail, fxSvc, transferSvc, "platform-wallet-123", "flutterwave")

	req := WithdrawRequest{
		WalletID:     "wallet-123",
		Reference:    "REF-123",
		FiatAmount:   decimal.NewFromInt(16000),
		FiatCurrency: "NGN",
	}

	_, err := svc.InitiateWithdrawal(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify withdrawal record was created and its status updated to Failed
	if len(repo.withdrawals) != 1 {
		t.Fatalf("expected 1 withdrawal in repo, got %d", len(repo.withdrawals))
	}

	var w *domain.FiatWithdrawal
	for _, val := range repo.withdrawals {
		w = val
		break
	}

	if w.Status != domain.FiatStatusFailed {
		t.Errorf("expected withdrawal status to be Failed, got %s", w.Status)
	}
}

func TestInitiateWithdrawal_RailFailure(t *testing.T) {
	repo := newMockRepository()
	rail := &mockRail{
		withdrawErr: errors.New("rail endpoint timeout"),
	}
	fxSvc := &mockFXService{
		quote: &fx.Quote{
			Rate: decimal.NewFromInt(1600),
		},
	}
	transferSvc := &mockTransferService{}

	svc := NewService(repo, rail, fxSvc, transferSvc, "platform-wallet-123", "flutterwave")

	req := WithdrawRequest{
		WalletID:     "wallet-123",
		Reference:    "REF-123",
		FiatAmount:   decimal.NewFromInt(16000),
		FiatCurrency: "NGN",
	}

	_, err := svc.InitiateWithdrawal(context.Background(), req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Verify withdrawal record was created and its status updated to Failed
	if len(repo.withdrawals) != 1 {
		t.Fatalf("expected 1 withdrawal in repo, got %d", len(repo.withdrawals))
	}

	var w *domain.FiatWithdrawal
	for _, val := range repo.withdrawals {
		w = val
		break
	}

	if w.Status != domain.FiatStatusFailed {
		t.Errorf("expected withdrawal status to be Failed, got %s", w.Status)
	}
}
