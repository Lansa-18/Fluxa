package fiat

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/fluxa/fluxa/internal/domain"
	"github.com/fluxa/fluxa/internal/fx"
	"github.com/fluxa/fluxa/internal/transfer"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Repository interface {
	CreateDeposit(ctx context.Context, d *domain.FiatDeposit) error
	UpdateDepositStatus(ctx context.Context, id, status string) error
	UpdateDepositInstructions(ctx context.Context, id string, instructions map[string]string) error
	GetDepositByReference(ctx context.Context, ref string) (*domain.FiatDeposit, error)
	GetDepositByID(ctx context.Context, id string) (*domain.FiatDeposit, error)
	CreateWithdrawal(ctx context.Context, w *domain.FiatWithdrawal) error
	UpdateWithdrawalStatus(ctx context.Context, id, status string) error
	GetWithdrawalByReference(ctx context.Context, ref string) (*domain.FiatWithdrawal, error)
	GetWithdrawalByID(ctx context.Context, id string) (*domain.FiatWithdrawal, error)
}

type Service interface {
	GetQuote(ctx context.Context, fiatCurrency, country, amount string) (*FiatQuote, error)
	InitiateDeposit(ctx context.Context, req DepositRequest) (*DepositResponse, error)
	InitiateWithdrawal(ctx context.Context, req WithdrawRequest) (*WithdrawResponse, error)
	HandleWebhook(ctx context.Context, provider string, payload []byte, headers map[string]string) error
}

type service struct {
	repo             Repository
	providers        map[string]Provider
	fxSvc            fx.Service
	transferSvc      transfer.Service
	platformWalletID string
}

func NewService(repo Repository, providers []Provider, fxSvc fx.Service, transferSvc transfer.Service, platformWalletID string) Service {
	pm := make(map[string]Provider, len(providers))
	for _, p := range providers {
		pm[p.Name()] = p
	}
	return &service{
		repo:             repo,
		providers:        pm,
		fxSvc:            fxSvc,
		transferSvc:      transferSvc,
		platformWalletID: platformWalletID,
	}
}

func (s *service) provider(name string) (Provider, error) {
	p, ok := s.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown fiat provider: %s", name)
	}
	return p, nil
}

func (s *service) GetQuote(ctx context.Context, fiatCurrency, country, amount string) (*FiatQuote, error) {
	fiatAmt, err := decimal.NewFromString(amount)
	if err != nil || fiatAmt.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("invalid amount")
	}

	for _, p := range s.providers {
		for _, c := range p.SupportedCountries() {
			if c == country {
				return p.GetQuote(ctx, QuoteRequest{
					FiatCurrency: fiatCurrency,
					FiatAmount:   fiatAmt,
					Country:      country,
				})
			}
		}
	}
	return nil, fmt.Errorf("no provider supports country %s", country)
}

func (s *service) InitiateDeposit(ctx context.Context, req DepositRequest) (*DepositResponse, error) {
	p, err := s.provider("flutterwave")
	if err != nil {
		return nil, err
	}

	quote, err := p.GetQuote(ctx, QuoteRequest{
		FiatCurrency: req.FiatCurrency,
		FiatAmount:   req.FiatAmount,
	})
	if err != nil {
		return nil, fmt.Errorf("get quote for deposit: %w", err)
	}

	deposit := &domain.FiatDeposit{
		ID:                uuid.New().String(),
		WalletID:          req.WalletID,
		Provider:          p.Name(),
		ProviderReference: req.Reference,
		FiatAmount:        req.FiatAmount,
		FiatCurrency:      req.FiatCurrency,
		USDCAmount:        quote.USDCAmount,
		Status:            domain.FiatStatusPending,
		CreatedAt:         time.Now().UTC(),
	}

	if err := s.repo.CreateDeposit(ctx, deposit); err != nil {
		return nil, fmt.Errorf("create deposit record: %w", err)
	}

	inst, err := p.InitiateDeposit(ctx, req)
	if err != nil {
		_ = s.repo.UpdateDepositStatus(ctx, deposit.ID, domain.FiatStatusFailed)
		return nil, fmt.Errorf("provider deposit error: %w", err)
	}

	if err := s.repo.UpdateDepositInstructions(ctx, deposit.ID, inst.Instructions); err != nil {
		return nil, fmt.Errorf("update deposit instructions: %w", err)
	}

	return &DepositResponse{
		PaymentLink: inst.Instructions["payment_link"],
		Reference:   inst.ProviderRef,
	}, nil
}

func (s *service) InitiateWithdrawal(ctx context.Context, req WithdrawRequest) (*WithdrawResponse, error) {
	p, err := s.provider("flutterwave")
	if err != nil {
		return nil, err
	}

	rate := decimal.NewFromInt(1500)
	usdcAmount := req.FiatAmount.Div(rate)

	withdrawal := &domain.FiatWithdrawal{
		ID:                uuid.New().String(),
		WalletID:          req.WalletID,
		Provider:          p.Name(),
		ProviderReference: req.Reference,
		FiatAmount:        req.FiatAmount,
		FiatCurrency:      req.FiatCurrency,
		USDCAmount:        usdcAmount,
		Status:            domain.FiatStatusPending,
		CreatedAt:         time.Now().UTC(),
	}

	if err := s.repo.CreateWithdrawal(ctx, withdrawal); err != nil {
		return nil, fmt.Errorf("create withdrawal record: %w", err)
	}

	_, err = s.transferSvc.InitiateTransfer(ctx, req.WalletID, s.platformWalletID, "USDC", usdcAmount)
	if err != nil {
		_ = s.repo.UpdateWithdrawalStatus(ctx, withdrawal.ID, domain.FiatStatusFailed)
		return nil, fmt.Errorf("initiate transfer to platform: %w", err)
	}

	wReq := WithdrawalRequest{
		WalletID:      req.WalletID,
		ProviderRef:   req.Reference,
		FiatAmount:    req.FiatAmount,
		FiatCurrency:  req.FiatCurrency,
		AccountBank:   req.AccountBank,
		AccountNumber: req.AccountNumber,
	}

	result, err := p.InitiateWithdrawal(ctx, wReq)
	if err != nil {
		_ = s.repo.UpdateWithdrawalStatus(ctx, withdrawal.ID, domain.FiatStatusFailed)
		return nil, fmt.Errorf("provider withdraw error: %w", err)
	}

	return &WithdrawResponse{
		Reference: result.ProviderRef,
		Status:    result.Status,
	}, nil
}

func (s *service) HandleWebhook(ctx context.Context, providerName string, payload []byte, headers map[string]string) error {
	p, err := s.provider(providerName)
	if err != nil {
		return err
	}

	httpHeaders := make(http.Header)
	for k, v := range headers {
		httpHeaders.Set(k, v)
	}

	evt, err := p.HandleWebhook(ctx, payload, httpHeaders)
	if err != nil {
		return fmt.Errorf("handle webhook: %w", err)
	}

	switch evt.Type {
	case EventDepositConfirmed:
		deposit, err := s.repo.GetDepositByReference(ctx, evt.ProviderRef)
		if err != nil {
			return fmt.Errorf("get deposit by ref: %w", err)
		}
		if deposit.Status != domain.FiatStatusPending {
			return nil
		}
		_, err = s.transferSvc.InitiateTransfer(ctx, s.platformWalletID, deposit.WalletID, "USDC", deposit.USDCAmount)
		if err != nil {
			return fmt.Errorf("credit user wallet: %w", err)
		}
		if err := s.repo.UpdateDepositStatus(ctx, deposit.ID, domain.FiatStatusCompleted); err != nil {
			return fmt.Errorf("update deposit status: %w", err)
		}

	case EventDepositFailed:
		deposit, err := s.repo.GetDepositByReference(ctx, evt.ProviderRef)
		if err != nil {
			return fmt.Errorf("get deposit by ref: %w", err)
		}
		if deposit.Status != domain.FiatStatusPending {
			return nil
		}
		if err := s.repo.UpdateDepositStatus(ctx, deposit.ID, domain.FiatStatusFailed); err != nil {
			return fmt.Errorf("update deposit status: %w", err)
		}

	case EventWithdrawalSent:
		withdrawal, err := s.repo.GetWithdrawalByReference(ctx, evt.ProviderRef)
		if err != nil {
			return fmt.Errorf("get withdrawal by ref: %w", err)
		}
		if withdrawal.Status != domain.FiatStatusPending {
			return nil
		}
		if err := s.repo.UpdateWithdrawalStatus(ctx, withdrawal.ID, domain.FiatStatusCompleted); err != nil {
			return fmt.Errorf("update withdrawal status: %w", err)
		}

	case EventWithdrawalFailed:
		withdrawal, err := s.repo.GetWithdrawalByReference(ctx, evt.ProviderRef)
		if err != nil {
			return fmt.Errorf("get withdrawal by ref: %w", err)
		}
		if withdrawal.Status != domain.FiatStatusPending {
			return nil
		}
		if err := s.repo.UpdateWithdrawalStatus(ctx, withdrawal.ID, domain.FiatStatusFailed); err != nil {
			return fmt.Errorf("update withdrawal status: %w", err)
		}
	}

	return nil
}
