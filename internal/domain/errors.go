package domain

import "errors"

var (
	ErrWalletNotFound      = errors.New("wallet not found")
	ErrTransactionNotFound = errors.New("transaction not found")
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrStellarSubmission   = errors.New("stellar transaction submission failed")
	ErrDecryptionFailed    = errors.New("secret key decryption failed")
	ErrSlippageExceeded    = errors.New("slippage tolerance exceeded")
	ErrInvalidAsset        = errors.New("invalid or unsupported asset")
	ErrSelfTransfer              = errors.New("source and destination wallets must differ")
	ErrFeeScheduleNotFound       = errors.New("fee schedule not found")
	ErrReconciliationFailed      = errors.New("reconciliation check failed")
	ErrWebhookNotFound           = errors.New("webhook endpoint not found")
	ErrWebhookDeliveryNotFound   = errors.New("webhook delivery not found")
	ErrFiatProviderNotFound      = errors.New("fiat provider not found")
	ErrFiatDepositNotFound       = errors.New("fiat deposit not found")
	ErrFiatWithdrawalNotFound    = errors.New("fiat withdrawal not found")
	ErrFiatQuoteExpired          = errors.New("fiat quote expired")
	ErrUnsupportedCountry        = errors.New("unsupported country for fiat provider")
	ErrFiatWebhookVerification   = errors.New("fiat webhook verification failed")
)
