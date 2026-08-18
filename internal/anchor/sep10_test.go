package anchor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/network"
	"github.com/stellar/go/txnbuild"
)

func buildTestChallenge(t *testing.T, serverKP, clientKP *keypair.Full, webAuthDomain, homeDomain string) string {
	t.Helper()
	tx, err := txnbuild.BuildChallengeTx(
		serverKP.Seed(),
		clientKP.Address(),
		webAuthDomain,
		homeDomain,
		network.TestNetworkPassphrase,
		300*time.Second,
		nil,
	)
	if err != nil {
		t.Fatalf("build challenge tx: %v", err)
	}
	xdrStr, err := tx.Base64()
	if err != nil {
		t.Fatalf("encode challenge tx: %v", err)
	}
	return xdrStr
}

func TestSep10Client_Challenge_RejectsWrongSourceAccount(t *testing.T) {
	realServerKP := keypair.MustRandom()
	imposterKP := keypair.MustRandom() // signs/sources the challenge but is NOT the anchor's declared signing key
	clientKP := keypair.MustRandom()

	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		// The anchor's stellar.toml claims SIGNING_KEY = realServerKP, but the
		// challenge it actually returns is sourced from a different account.
		challengeXDR := buildTestChallenge(t, imposterKP, clientKP, r.Host, "example.com")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"transaction":        challengeXDR,
			"network_passphrase": network.TestNetworkPassphrase,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewSep10Client(srv.URL+"/auth", "example.com", network.TestNetworkPassphrase, nil)

	jwt, err := client.Challenge(context.Background(), realServerKP.Address(), clientKP.Address())
	if err == nil {
		t.Fatalf("expected Challenge to reject a challenge not sourced from the anchor's signing key, got challenge %q with no error", jwt)
	}
	if jwt != "" {
		t.Fatalf("expected no challenge to be returned on validation failure, got %q", jwt)
	}
}

func TestSep10Client_ChallengeAndAuthenticate_Success(t *testing.T) {
	serverKP := keypair.MustRandom()
	clientKP := keypair.MustRandom()

	var challengeXDR string
	var authenticatedToken = "sep10-jwt-token"

	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			webAuthDomain := r.Host
			challengeXDR = buildTestChallenge(t, serverKP, clientKP, webAuthDomain, "example.com")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"transaction":        challengeXDR,
				"network_passphrase": network.TestNetworkPassphrase,
			})
		case http.MethodPost:
			var body struct {
				Transaction string `json:"transaction"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Transaction == "" {
				t.Errorf("expected a signed transaction in the auth request body")
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": authenticatedToken})
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := NewSep10Client(srv.URL+"/auth", "example.com", network.TestNetworkPassphrase, nil)

	challenge, err := client.Challenge(context.Background(), serverKP.Address(), clientKP.Address())
	if err != nil {
		t.Fatalf("Challenge returned unexpected error: %v", err)
	}
	if challenge == "" {
		t.Fatalf("expected a non-empty challenge")
	}

	token, err := client.Authenticate(context.Background(), challenge, clientKP.Seed())
	if err != nil {
		t.Fatalf("Authenticate returned unexpected error: %v", err)
	}
	if token != authenticatedToken {
		t.Fatalf("expected token %q, got %q", authenticatedToken, token)
	}
}
