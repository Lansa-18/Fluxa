package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func postVerify(t *testing.T, h *Handler, body verifyRequest) (*httptest.ResponseRecorder, verifyResponse) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/verify", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	h.Verify(rec, req)

	var resp verifyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (body=%s)", err, rec.Body.String())
	}
	return rec, resp
}

func TestHandlerVerify_ValidSignature(t *testing.T) {
	h := NewHandler(nil) // Verify doesn't touch the service
	secret := "whsec_test"
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	body := `{"event":"transfer.settled"}`
	sig := sign(secret, timestamp, []byte(body))

	rec, resp := postVerify(t, h, verifyRequest{Secret: secret, Timestamp: timestamp, Body: body, Signature: sig})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !resp.Valid {
		t.Fatalf("expected valid=true, got reason=%v", resp.Reason)
	}
	if resp.Reason != nil {
		t.Fatalf("expected nil reason on success, got %v", *resp.Reason)
	}
}

func TestHandlerVerify_StaleTimestampReturnsReason(t *testing.T) {
	h := NewHandler(nil)
	secret := "whsec_test"
	timestamp := strconv.FormatInt(time.Now().Add(-6*time.Minute).Unix(), 10)
	body := `{"event":"transfer.failed"}`
	sig := sign(secret, timestamp, []byte(body))

	rec, resp := postVerify(t, h, verifyRequest{Secret: secret, Timestamp: timestamp, Body: body, Signature: sig})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (verify reports validity in the body, not via HTTP status)", rec.Code)
	}
	if resp.Valid {
		t.Fatal("expected valid=false for a 6-minute-old timestamp")
	}
	if resp.Reason == nil || *resp.Reason != "stale_timestamp" {
		t.Fatalf("reason = %v, want stale_timestamp", resp.Reason)
	}
}

func TestHandlerVerify_MissingFieldReturns400(t *testing.T) {
	h := NewHandler(nil)
	raw, _ := json.Marshal(map[string]string{"secret": "s"}) // missing timestamp/signature
	req := httptest.NewRequest(http.MethodPost, "/v1/webhooks/verify", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	h.Verify(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
