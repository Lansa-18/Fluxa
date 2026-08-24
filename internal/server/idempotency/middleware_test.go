package idempotency_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/fluxa/fluxa/internal/server/idempotency"
	"github.com/fluxa/fluxa/internal/tenant"
	"github.com/google/uuid"
)

// mockRepo is a minimal in-memory Repository that mirrors the real
// Postgres-backed semantics closely enough to exercise the middleware:
// the first TryAcquire for a (org, key) pair wins and the record starts
// "processing"; every subsequent TryAcquire sees the same record until
// Complete overwrites it.
type mockRepo struct {
	mu      sync.Mutex
	records map[string]*idempotency.Record
}

func newMockRepo() *mockRepo {
	return &mockRepo{records: map[string]*idempotency.Record{}}
}

func (m *mockRepo) TryAcquire(ctx context.Context, orgID, key, requestHash string, expiresAt time.Time) (*idempotency.Record, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := orgID + ":" + key
	if rec, ok := m.records[k]; ok {
		cp := *rec
		return &cp, true, nil
	}
	m.records[k] = &idempotency.Record{OrgID: orgID, Key: key, RequestHash: requestHash, Status: idempotency.StatusProcessing}
	return nil, false, nil
}

func (m *mockRepo) Complete(ctx context.Context, orgID, key string, responseStatus int, responseBody []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := orgID + ":" + key
	rec, ok := m.records[k]
	if !ok {
		return nil
	}
	rec.Status = idempotency.StatusComplete
	rec.ResponseStatus = responseStatus
	rec.ResponseBody = responseBody
	return nil
}

func newRequest(t *testing.T, key, body string) *http.Request {
	t.Helper()
	ctx := tenant.WithID(context.Background(), "org-1")
	req := httptest.NewRequest(http.MethodPost, "/v1/transfers", bytes.NewBufferString(body)).WithContext(ctx)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	return req
}

func decodeErrorCode(t *testing.T, body []byte) string {
	t.Helper()
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode error response: %v (body=%s)", err, body)
	}
	return resp.Error.Code
}

func TestMissingKeyReturns400(t *testing.T) {
	repo := newMockRepo()
	mw := idempotency.Middleware(repo)

	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newRequest(t, "", `{"a":1}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if code := decodeErrorCode(t, rec.Body.Bytes()); code != "IDEMPOTENCY_KEY_REQUIRED" {
		t.Fatalf("expected IDEMPOTENCY_KEY_REQUIRED, got %s", code)
	}
	if called {
		t.Fatal("handler should not have been called")
	}
}

func TestInvalidKeyFormatReturns400(t *testing.T) {
	repo := newMockRepo()
	mw := idempotency.Middleware(repo)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newRequest(t, "not-a-uuid", `{}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if code := decodeErrorCode(t, rec.Body.Bytes()); code != "INVALID_IDEMPOTENCY_KEY_FORMAT" {
		t.Fatalf("expected INVALID_IDEMPOTENCY_KEY_FORMAT, got %s", code)
	}
}

func TestSameKeySameBodyReplaysResponseByteForByte(t *testing.T) {
	repo := newMockRepo()
	mw := idempotency.Middleware(repo)

	callCount := 0
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"tx-1","status":"pending"}`))
	}))

	key := uuid.New().String()
	body := `{"from_wallet_id":"a","to_wallet_id":"b","asset":"XLM","amount":"10"}`

	first := httptest.NewRecorder()
	h.ServeHTTP(first, newRequest(t, key, body))

	second := httptest.NewRecorder()
	h.ServeHTTP(second, newRequest(t, key, body))

	if callCount != 1 {
		t.Fatalf("expected handler to run exactly once, ran %d times", callCount)
	}
	if first.Code != second.Code || first.Body.String() != second.Body.String() {
		t.Fatalf("responses differ: first=%d %q second=%d %q", first.Code, first.Body.String(), second.Code, second.Body.String())
	}
	if second.Code != http.StatusAccepted || second.Body.String() != `{"id":"tx-1","status":"pending"}` {
		t.Fatalf("unexpected replayed response: %d %s", second.Code, second.Body.String())
	}
}

func TestSameKeyDifferentBodyReturns422(t *testing.T) {
	repo := newMockRepo()
	mw := idempotency.Middleware(repo)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	key := uuid.New().String()
	first := httptest.NewRecorder()
	h.ServeHTTP(first, newRequest(t, key, `{"amount":"10"}`))

	second := httptest.NewRecorder()
	h.ServeHTTP(second, newRequest(t, key, `{"amount":"20"}`))

	if second.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", second.Code)
	}
	if code := decodeErrorCode(t, second.Body.Bytes()); code != "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_BODY" {
		t.Fatalf("expected IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_BODY, got %s", code)
	}
}

func TestConcurrentRequestInProgressReturns409(t *testing.T) {
	repo := newMockRepo()
	key := uuid.New().String()
	// Simulate a winning concurrent request that has already claimed the key
	// and is still executing.
	repo.records["org-1:"+key] = &idempotency.Record{OrgID: "org-1", Key: key, Status: idempotency.StatusProcessing}

	mw := idempotency.Middleware(repo)
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newRequest(t, key, `{"amount":"10"}`))

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
	if code := decodeErrorCode(t, rec.Body.Bytes()); code != "REQUEST_IN_PROGRESS" {
		t.Fatalf("expected REQUEST_IN_PROGRESS, got %s", code)
	}
	if called {
		t.Fatal("handler should not run for a request that lost the race")
	}
}

func TestHandlerStillReceivesRequestBody(t *testing.T) {
	repo := newMockRepo()
	mw := idempotency.Middleware(repo)

	var received string
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		received = buf.String()
		w.WriteHeader(http.StatusOK)
	}))

	body := `{"amount":"42"}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, newRequest(t, uuid.New().String(), body))

	if received != body {
		t.Fatalf("expected handler to read original body %q, got %q", body, received)
	}
}
