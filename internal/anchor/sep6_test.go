package anchor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSep6Client_InitiateDeposit_ReturnsBankInstructions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/deposit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET per SEP-6, got %s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-jwt" {
			t.Errorf("expected bearer jwt, got %q", got)
		}
		q := r.URL.Query()
		if q.Get("asset_code") != "USDC" {
			t.Errorf("expected asset_code=USDC, got %q", q.Get("asset_code"))
		}
		if q.Get("account") == "" {
			t.Errorf("expected account to be set")
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":  "deposit-tx-1",
			"how": "Make a payment to Bank: 121122676 Account: 13719713158835300",
			"eta": 1800,
			"instructions": map[string]interface{}{
				"organization.bank_number": map[string]string{
					"value":       "121122676",
					"description": "US bank routing number",
				},
				"organization.bank_account_number": map[string]string{
					"value":       "13719713158835300",
					"description": "US bank account number",
				},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewSep6Client(nil)
	instr, err := client.InitiateDeposit(context.Background(), srv.URL, "test-jwt", "USDC", "GACCOUNT", "100", "user@example.com")
	if err != nil {
		t.Fatalf("InitiateDeposit returned unexpected error: %v", err)
	}
	if instr.ID != "deposit-tx-1" {
		t.Fatalf("expected id deposit-tx-1, got %q", instr.ID)
	}
	if len(instr.Instructions) != 2 {
		t.Fatalf("expected 2 instruction fields, got %d", len(instr.Instructions))
	}
	if instr.Instructions["organization.bank_number"].Value != "121122676" {
		t.Fatalf("unexpected bank number instruction: %+v", instr.Instructions["organization.bank_number"])
	}
}

func TestSep6Client_GetInfo_ParsesDepositWithdrawSupport(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"deposit": map[string]interface{}{
				"USDC": map[string]interface{}{"enabled": true, "fee_fixed": 0.5, "min_amount": 1},
			},
			"withdraw": map[string]interface{}{
				"USDC": map[string]interface{}{"enabled": true, "fee_percent": 1.5},
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewSep6Client(nil)
	info, err := client.GetInfo(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("GetInfo returned unexpected error: %v", err)
	}
	if !info.Deposit["USDC"].Enabled {
		t.Fatalf("expected USDC deposit enabled")
	}
	if !info.Withdraw["USDC"].Enabled {
		t.Fatalf("expected USDC withdraw enabled")
	}
}

func TestSep6Client_GetTransaction_ReflectsCompletedStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/transaction", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "deposit-tx-1" {
			t.Errorf("expected id=deposit-tx-1, got %q", r.URL.Query().Get("id"))
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"transaction": map[string]interface{}{
				"id":                     "deposit-tx-1",
				"kind":                   "deposit",
				"status":                 "completed",
				"amount_in":              "100.00",
				"stellar_transaction_id": "abc123",
			},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewSep6Client(nil)
	tx, err := client.GetTransaction(context.Background(), srv.URL, "test-jwt", "deposit-tx-1")
	if err != nil {
		t.Fatalf("GetTransaction returned unexpected error: %v", err)
	}
	if tx.Status != "completed" {
		t.Fatalf("expected status completed, got %q", tx.Status)
	}
}
