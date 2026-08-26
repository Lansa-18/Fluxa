// Package idempotency implements request deduplication for state-mutating
// endpoints via the Idempotency-Key header, following the same pattern as
// Stripe/Adyen: a client-supplied UUID v4 key scopes a request so that a
// retry (e.g. after a timed-out response) replays the original result
// instead of re-executing the handler.
package idempotency

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"github.com/fluxa/fluxa/internal/api"
	"github.com/fluxa/fluxa/internal/tenant"
	"github.com/google/uuid"
)

const (
	headerKey = "Idempotency-Key"
	ttl       = 24 * time.Hour
)

// Middleware returns chi/http middleware that enforces idempotency-key
// semantics for the route(s) it wraps. It is applied selectively — only to
// the state-mutating routes that require it — by each handler package's
// Routes() method, not globally.
func Middleware(repo Repository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(headerKey)
			if key == "" {
				api.Error(w, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required for this endpoint")
				return
			}

			parsed, err := uuid.Parse(key)
			if err != nil || parsed.Version() != 4 {
				api.Error(w, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY_FORMAT", "Idempotency-Key must be a valid UUID v4")
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				api.BadRequest(w, "failed to read request body")
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			hash := requestHash(r.Method, r.URL.Path, body)
			orgID := tenant.IDFromContext(r.Context())

			rec, existed, err := repo.TryAcquire(r.Context(), orgID, key, hash, time.Now().UTC().Add(ttl))
			if err != nil {
				api.InternalError(w, err)
				return
			}

			if existed {
				switch {
				case rec.Status == StatusProcessing:
					api.Error(w, http.StatusConflict, "REQUEST_IN_PROGRESS", "a request with this idempotency key is already being processed")
				case rec.RequestHash != hash:
					api.UnprocessableEntity(w, "IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_BODY", "this idempotency key was previously used with a different request body")
				default:
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(rec.ResponseStatus)
					_, _ = w.Write(rec.ResponseBody)
				}
				return
			}

			rec2 := newRecorder(w)
			next.ServeHTTP(rec2, r)
			_ = repo.Complete(r.Context(), orgID, key, rec2.status, rec2.body)
		})
	}
}

func requestHash(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte(path))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}
