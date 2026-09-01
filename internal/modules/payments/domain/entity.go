package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type TransactionStatus string

const (
	TxInitiated         TransactionStatus = "initiated"
	TxAuthorized        TransactionStatus = "authorized"
	TxCaptured          TransactionStatus = "captured"
	TxFailed            TransactionStatus = "failed"
	TxRefunded          TransactionStatus = "refunded"
	TxPartiallyRefunded TransactionStatus = "partially_refunded"
)

type Method string

const (
	MethodCOD    Method = "cod"
	MethodUPI    Method = "upi"
	MethodCard   Method = "card"
	MethodWallet Method = "wallet"
)

type RefundStatus string

const (
	RefundPending    RefundStatus = "pending"
	RefundProcessing RefundStatus = "processing"
	RefundCompleted  RefundStatus = "completed"
	RefundFailed     RefundStatus = "failed"
)

type Transaction struct {
	ID                uuid.UUID
	OrderID           uuid.UUID
	CustomerID        uuid.UUID
	Amount            float64
	Currency          string
	Method            Method
	Status            TransactionStatus
	Gateway           string
	GatewayOrderID    *string
	GatewayPaymentID  *string
	FailureReason     *string
	InitiatedAt       time.Time
	CompletedAt       *time.Time
}

func (t *Transaction) IsSettled() bool {
	return t.Status == TxCaptured
}

type SavedMethod struct {
	ID            uuid.UUID
	CustomerID    uuid.UUID
	Method        Method
	Gateway       string
	GatewayToken  string
	DisplayLabel  string
	IsDefault     bool
	CreatedAt     time.Time
}

type Refund struct {
	ID                    uuid.UUID
	PaymentTransactionID  uuid.UUID
	OrderID               uuid.UUID
	Amount                float64
	Reason                string
	Status                RefundStatus
	GatewayRefundID       *string
	InitiatedBy           *uuid.UUID
	FailureReason         *string
	InitiatedAt           time.Time
	CompletedAt           *time.Time
}

// InitiateResult is what the client (Flutter apps) needs to open the
// gateway's checkout SDK/redirect flow.
type InitiateResult struct {
	Transaction    *Transaction
	GatewayOrderID string
	GatewayKey     string // publishable/public key the client SDK needs, NOT a secret
	Amount         float64
	Currency       string
}

// CaptureCallback is what the client sends back after completing payment
// in the gateway's SDK, to be verified server-side before trusting it.
type CaptureCallback struct {
	GatewayOrderID   string
	GatewayPaymentID string
	GatewaySignature string
}

// PaymentGateway abstracts the actual payment provider (Razorpay,
// Stripe, PayU, ...) so application code never imports a
// provider-specific SDK directly — matches the master spec's requirement
// for an abstracted payments interface. Swapping providers means writing
// a new implementation of this interface, not touching AppService.
type PaymentGateway interface {
	Name() string
	// CreateOrder opens a payment intent/order on the gateway's side,
	// returning the gateway's own order ID and the amount actually
	// registered (in case the gateway rounds/adjusts).
	CreateOrder(ctx context.Context, amount float64, currency string, receiptID string) (gatewayOrderID string, err error)
	// VerifyCallback checks the client-provided signature against the
	// gateway's HMAC scheme, returning an error if it doesn't match —
	// this is the step that prevents a malicious client from just
	// POSTing "payment succeeded" without actually paying.
	VerifyCallback(cb CaptureCallback) error
	// Refund issues a refund for a previously captured payment.
	Refund(ctx context.Context, gatewayPaymentID string, amount float64, reason string) (gatewayRefundID string, err error)
}

// Repository is the persistence port for the Payments module.
type Repository interface {
	CreateTransaction(ctx context.Context, t *Transaction) (*Transaction, error)
	GetTransactionByID(ctx context.Context, id uuid.UUID) (*Transaction, error)
	GetTransactionByGatewayOrderID(ctx context.Context, gateway, gatewayOrderID string) (*Transaction, error)
	ListTransactionsByOrder(ctx context.Context, orderID uuid.UUID) ([]*Transaction, error)
	GetLatestTransactionForOrder(ctx context.Context, orderID uuid.UUID) (*Transaction, error)
	MarkCaptured(ctx context.Context, id uuid.UUID, gatewayPaymentID, signature string) (*Transaction, error)
	MarkFailed(ctx context.Context, id uuid.UUID, reason string) (*Transaction, error)
	MarkAuthorized(ctx context.Context, id uuid.UUID, gatewayPaymentID string) (*Transaction, error)
	SetStatus(ctx context.Context, id uuid.UUID, status TransactionStatus) error

	CreateSavedMethod(ctx context.Context, m *SavedMethod) (*SavedMethod, error)
	ListSavedMethods(ctx context.Context, customerID uuid.UUID) ([]*SavedMethod, error)
	UnsetDefaultMethods(ctx context.Context, customerID uuid.UUID) error
	DeleteSavedMethod(ctx context.Context, id, customerID uuid.UUID) error

	CreateRefund(ctx context.Context, r *Refund) (*Refund, error)
	GetRefundByID(ctx context.Context, id uuid.UUID) (*Refund, error)
	ListRefundsByOrder(ctx context.Context, orderID uuid.UUID) ([]*Refund, error)
	MarkRefundCompleted(ctx context.Context, id uuid.UUID, gatewayRefundID string) (*Refund, error)
	MarkRefundFailed(ctx context.Context, id uuid.UUID, reason string) (*Refund, error)
	SumCompletedRefundsForOrder(ctx context.Context, orderID uuid.UUID) (float64, error)
}
