package settlement

import (
	"testing"

	"github.com/stellar/go/txnbuild"
)

func TestBuildAsset_XLM(t *testing.T) {
	e := &Engine{assetIssuers: map[string]string{"USDC": "usdc-issuer"}}
	asset, err := e.buildAsset("XLM")
	if err != nil {
		t.Fatalf("buildAsset(XLM) error: %v", err)
	}
	if _, ok := asset.(txnbuild.NativeAsset); !ok {
		t.Fatalf("expected NativeAsset, got %T", asset)
	}
}

func TestBuildAsset_USDC(t *testing.T) {
	e := &Engine{assetIssuers: map[string]string{"USDC": "usdc-issuer-123"}}
	asset, err := e.buildAsset("USDC")
	if err != nil {
		t.Fatalf("buildAsset(USDC) error: %v", err)
	}
	ca, ok := asset.(txnbuild.CreditAsset)
	if !ok {
		t.Fatalf("expected CreditAsset, got %T", asset)
	}
	if ca.Code != "USDC" {
		t.Fatalf("code = %q, want USDC", ca.Code)
	}
	if ca.Issuer != "usdc-issuer-123" {
		t.Fatalf("issuer = %q, want usdc-issuer-123", ca.Issuer)
	}
}

func TestBuildAsset_EURC(t *testing.T) {
	e := &Engine{assetIssuers: map[string]string{
		"USDC": "usdc-issuer",
		"EURC": "eurc-issuer-456",
	}}
	asset, err := e.buildAsset("EURC")
	if err != nil {
		t.Fatalf("buildAsset(EURC) error: %v", err)
	}
	ca, ok := asset.(txnbuild.CreditAsset)
	if !ok {
		t.Fatalf("expected CreditAsset, got %T", asset)
	}
	if ca.Code != "EURC" {
		t.Fatalf("code = %q, want EURC", ca.Code)
	}
	if ca.Issuer != "eurc-issuer-456" {
		t.Fatalf("issuer = %q, want eurc-issuer-456", ca.Issuer)
	}
}

func TestBuildAsset_Unsupported(t *testing.T) {
	e := &Engine{assetIssuers: map[string]string{"USDC": "usdc-issuer"}}
	_, err := e.buildAsset("DOGE")
	if err == nil {
		t.Fatal("expected error for unsupported asset, got nil")
	}
}

func TestBuildAsset_EmptyRegistry(t *testing.T) {
	e := &Engine{assetIssuers: map[string]string{}}
	_, err := e.buildAsset("USDC")
	if err == nil {
		t.Fatal("expected error for USDC with empty registry, got nil")
	}
}
