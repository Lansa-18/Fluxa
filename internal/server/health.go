package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const healthCheckTimeout = 2 * time.Second
const healthStatusCacheTTL = 5 * time.Second

type DependencyCheck func(context.Context) error

func HorizonDependencyCheck(baseURL string) DependencyCheck {
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/fee_stats", nil)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("unexpected status %d", resp.StatusCode)
		}
		var payload struct {
			LastLedgerBaseFee int64 `json:"last_ledger_base_fee"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			return err
		}
		if payload.LastLedgerBaseFee <= 0 {
			return fmt.Errorf("last_ledger_base_fee is not positive")
		}
		return nil
	}
}

func HTTPDependencyCheck(url string) DependencyCheck {
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("unexpected status %d", resp.StatusCode)
		}
		return nil
	}
}

type dependencyHealth struct {
	Status         string `json:"status"`
	ResponseTimeMS int64  `json:"response_time_ms"`
	Error          string `json:"error,omitempty"`
}

type healthResponse struct {
	Status string                      `json:"status"`
	Checks map[string]dependencyHealth `json:"checks,omitempty"`
}

type cachedHealthResponse struct {
	statusCode int
	body       healthResponse
	expiresAt  time.Time
}

func HealthHandler(checks map[string]DependencyCheck) http.HandlerFunc {
	var mu sync.Mutex
	var cached cachedHealthResponse

	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		mu.Lock()
		if now.Before(cached.expiresAt) {
			writeHealthResponse(w, cached.statusCode, cached.body)
			mu.Unlock()
			return
		}
		mu.Unlock()

		ctx, cancel := context.WithTimeout(r.Context(), healthCheckTimeout)
		defer cancel()

		status := http.StatusOK
		resp := healthResponse{
			Status: "ok",
			Checks: make(map[string]dependencyHealth, len(checks)),
		}

		for name, check := range checks {
			start := time.Now()
			if err := check(ctx); err != nil {
				status = http.StatusServiceUnavailable
				resp.Status = "degraded"
				resp.Checks[name] = dependencyHealth{
					Status:         "unavailable",
					ResponseTimeMS: time.Since(start).Milliseconds(),
					Error:          err.Error(),
				}
				continue
			}
			resp.Checks[name] = dependencyHealth{
				Status:         "connected",
				ResponseTimeMS: time.Since(start).Milliseconds(),
			}
		}

		mu.Lock()
		cached = cachedHealthResponse{
			statusCode: status,
			body:       resp,
			expiresAt:  now.Add(healthStatusCacheTTL),
		}
		mu.Unlock()

		writeHealthResponse(w, status, resp)
	}
}

func writeHealthResponse(w http.ResponseWriter, status int, resp healthResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}
