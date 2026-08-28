package flutterwave

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fluxa/fluxa/internal/fiat"
)

type Provider struct {
	secretKey   string
	webhookHash string
	client      *http.Client
	baseURL     string
}

func NewProvider(secretKey, webhookHash string) *Provider {
	return &Provider{
		secretKey:   secretKey,
		webhookHash: webhookHash,
		client:      &http.Client{},
		baseURL:     "https://api.flutterwave.com/v3",
	}
}

func (p *Provider) Name() string {
	return "flutterwave"
}

func (p *Provider) SupportedCountries() []string {
	return []string{"NG", "GH", "KE", "ZA", "UG", "TZ"}
}

func (p *Provider) GetQuote(ctx context.Context, req fiat.QuoteRequest) (*fiat.FiatQuote, error) {
	if p.secretKey == "mock" || p.secretKey == "" {
		rate := decimal.NewFromInt(1500)
		usdcAmt := req.FiatAmount.Div(rate)
		return &fiat.FiatQuote{
			Provider:     "flutterwave",
			FiatAmount:   req.FiatAmount,
			FiatCurrency: req.FiatCurrency,
			USDCAmount:   usdcAmt,
			Rate:         rate,
			Fee:          decimal.NewFromInt(0),
			ExpiresAt:    time.Now().Add(30 * time.Second),
		}, nil
	}
	return nil, fmt.Errorf("flutterwave: GetQuote not yet implemented for production")
}

func (p *Provider) InitiateDeposit(ctx context.Context, req fiat.DepositRequest) (*fiat.DepositInstruction, error) {
	if p.secretKey == "mock" || p.secretKey == "" {
		return &fiat.DepositInstruction{
			ProviderRef: req.Reference,
			Instructions: map[string]string{
				"payment_link": fmt.Sprintf("https://mock.flutterwave.com/pay/%s", req.Reference),
			},
		}, nil
	}

	payload := map[string]interface{}{
		"tx_ref":       req.Reference,
		"amount":       req.FiatAmount.String(),
		"currency":     req.FiatCurrency,
		"redirect_url": "https://fluxa.io/payment/callback",
		"customer": map[string]string{
			"email": req.CustomerEmail,
			"name":  req.CustomerName,
		},
	}
	body, _ := json.Marshal(payload)

	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/payments", bytes.NewBuffer(body))
	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("flutterwave deposit api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from flutterwave: %d", resp.StatusCode)
	}

	var result struct {
		Status string `json:"status"`
		Data   struct {
			Link string `json:"link"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &fiat.DepositInstruction{
		ProviderRef: req.Reference,
		Instructions: map[string]string{
			"payment_link": result.Data.Link,
		},
	}, nil
}

func (p *Provider) InitiateWithdrawal(ctx context.Context, req fiat.WithdrawalRequest) (*fiat.WithdrawalResult, error) {
	if p.secretKey == "mock" || p.secretKey == "" {
		return &fiat.WithdrawalResult{
			ProviderRef: req.ProviderRef,
			Status:      "pending",
		}, nil
	}

	payload := map[string]interface{}{
		"account_bank":   req.AccountBank,
		"account_number": req.AccountNumber,
		"amount":         req.FiatAmount.String(),
		"currency":       req.FiatCurrency,
		"reference":      req.ProviderRef,
		"narration":      "Fluxa Withdrawal",
	}
	body, _ := json.Marshal(payload)

	httpReq, _ := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/transfers", bytes.NewBuffer(body))
	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("flutterwave withdraw api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from flutterwave transfer: %d", resp.StatusCode)
	}

	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Data    struct {
			Status    string `json:"status"`
			Reference string `json:"reference"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &fiat.WithdrawalResult{
		ProviderRef: result.Data.Reference,
		Status:      result.Data.Status,
	}, nil
}

func (p *Provider) GetStatus(ctx context.Context, providerRef string) (*fiat.RailEvent, error) {
	httpReq, _ := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/transfers/"+providerRef, nil)
	httpReq.Header.Set("Authorization", "Bearer "+p.secretKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("flutterwave get status: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
		Data   struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	evt := &fiat.RailEvent{
		ProviderRef: providerRef,
		Status:      "failed",
	}
	if result.Data.Status == "successful" {
		evt.Status = "completed"
	}
	return evt, nil
}

func (p *Provider) HandleWebhook(ctx context.Context, payload []byte, headers http.Header) (*fiat.RailEvent, error) {
	signature := headers.Get("verif-hash")
	if p.webhookHash != "" && p.webhookHash != "mock" {
		if signature != p.webhookHash {
			return nil, fmt.Errorf("invalid webhook signature")
		}
	}

	var data struct {
		Event string `json:"event"`
		Data  struct {
			TxRef     string  `json:"tx_ref"`
			Status    string  `json:"status"`
			Amount    float64 `json:"amount"`
			Reference string  `json:"reference"`
			Currency  string  `json:"currency"`
		} `json:"data"`
	}

	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("parse webhook payload: %w", err)
	}

	evt := &fiat.RailEvent{
		ProviderRef: data.Data.TxRef,
		Status:      "failed",
	}
	if evt.ProviderRef == "" {
		evt.ProviderRef = data.Data.Reference
	}
	evt.Amount = decimal.NewFromFloat(data.Data.Amount)
	evt.Currency = data.Data.Currency

	if data.Data.Status == "successful" {
		evt.Status = "completed"
	}

	switch data.Event {
	case "charge.completed":
		evt.Type = fiat.EventDepositConfirmed
	case "transfer.completed":
		evt.Type = fiat.EventWithdrawalSent
	default:
		evt.Type = data.Event
	}

	return evt, nil
}
