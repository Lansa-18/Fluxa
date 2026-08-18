package anchor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/stellar/go/txnbuild"
)

// Sep10Client performs SEP-10 web authentication against a single anchor's
// WEB_AUTH_ENDPOINT, proving control of a Stellar account to obtain a JWT.
type Sep10Client struct {
	webAuthEndpoint   string
	homeDomain        string
	networkPassphrase string
	httpClient        *http.Client
}

func NewSep10Client(webAuthEndpoint, homeDomain, networkPassphrase string, httpClient *http.Client) *Sep10Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Sep10Client{
		webAuthEndpoint:   webAuthEndpoint,
		homeDomain:        homeDomain,
		networkPassphrase: networkPassphrase,
		httpClient:        httpClient,
	}
}

type challengeResponse struct {
	Transaction       string `json:"transaction"`
	NetworkPassphrase string `json:"network_passphrase"`
	Error             string `json:"error"`
}

// Challenge requests a SEP-10 challenge transaction from the anchor and
// validates it before returning: the transaction must be signed by the
// anchor's serverSigningKey (the account named in SIGNING_KEY on its
// stellar.toml), have a zero sequence number, and its first ManageData
// operation must be sourced from clientPublicKey and named
// "<home domain> auth". Any anchor that returns a challenge sourced from a
// different account, or otherwise malformed, is rejected here and no JWT is
// ever requested for it.
func (c *Sep10Client) Challenge(ctx context.Context, serverSigningKey, clientPublicKey string) (string, error) {
	endpoint, err := url.Parse(c.webAuthEndpoint)
	if err != nil {
		return "", fmt.Errorf("parse web auth endpoint: %w", err)
	}
	q := endpoint.Query()
	q.Set("account", clientPublicKey)
	if c.homeDomain != "" {
		q.Set("home_domain", c.homeDomain)
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("build challenge request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request challenge: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read challenge response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("web_auth challenge request failed with status %d: %s", resp.StatusCode, body)
	}

	var chal challengeResponse
	if err := json.Unmarshal(body, &chal); err != nil {
		return "", fmt.Errorf("parse challenge response: %w", err)
	}
	if chal.Error != "" {
		return "", fmt.Errorf("anchor returned error: %s", chal.Error)
	}
	if chal.Transaction == "" {
		return "", fmt.Errorf("anchor challenge response has no transaction")
	}

	webAuthDomain := endpoint.Host

	// ReadChallengeTx verifies the transaction is sourced from and signed by
	// serverSigningKey, has sequence number 0, and that the first ManageData
	// operation is sourced from clientPublicKey and named "<home domain> auth".
	// A challenge whose source account is not the anchor's signing key fails
	// here with an error, and Authenticate is never called.
	_, txClientAccountID, _, _, err := txnbuild.ReadChallengeTx(
		chal.Transaction,
		serverSigningKey,
		c.networkPassphrase,
		webAuthDomain,
		[]string{c.homeDomain},
	)
	if err != nil {
		return "", fmt.Errorf("invalid SEP-10 challenge: %w", err)
	}
	if txClientAccountID != clientPublicKey {
		return "", fmt.Errorf("challenge client account %s does not match requested account %s", txClientAccountID, clientPublicKey)
	}

	return chal.Transaction, nil
}

type authRequest struct {
	Transaction string `json:"transaction"`
}

type authResponse struct {
	Token string `json:"token"`
	Error string `json:"error"`
}

// Authenticate signs a challenge previously returned by Challenge with the
// client's secret key and submits it back to the anchor, returning the JWT
// the anchor issues in exchange.
func (c *Sep10Client) Authenticate(ctx context.Context, challenge, clientSecretKey string) (string, error) {
	genericTx, err := txnbuild.TransactionFromXDR(challenge)
	if err != nil {
		return "", fmt.Errorf("parse challenge xdr: %w", err)
	}
	tx, ok := genericTx.Transaction()
	if !ok {
		return "", fmt.Errorf("challenge is not a valid transaction envelope")
	}

	signedTx, err := tx.SignWithKeyString(c.networkPassphrase, clientSecretKey)
	if err != nil {
		return "", fmt.Errorf("sign challenge: %w", err)
	}

	signedXDR, err := signedTx.Base64()
	if err != nil {
		return "", fmt.Errorf("encode signed challenge: %w", err)
	}

	reqBody, err := json.Marshal(authRequest{Transaction: signedXDR})
	if err != nil {
		return "", fmt.Errorf("marshal auth request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.webAuthEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("build auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("submit signed challenge: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read auth response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("web_auth token request failed with status %d: %s", resp.StatusCode, body)
	}

	var auth authResponse
	if err := json.Unmarshal(body, &auth); err != nil {
		return "", fmt.Errorf("parse auth response: %w", err)
	}
	if auth.Error != "" {
		return "", fmt.Errorf("anchor returned error: %s", auth.Error)
	}
	if auth.Token == "" {
		return "", fmt.Errorf("anchor did not return a token")
	}

	return auth.Token, nil
}
