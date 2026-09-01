// Package application orchestrates Payments use cases. It never talks to
// a gateway SDK directly — everything goes through domain.PaymentGateway
// — and it never writes to the orders table directly either, going
// through the small OrderReader/OrderPaymentUpdater interfaces instead
// (same cross-module pattern established in orders/application).
package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	notificationsdomain "github.com/foodapp/backend/internal/modules/notifications/domain"
	ordersdomain "github.com/foodapp/backend/internal/modules/orders/domain"
	"github.com/foodapp/backend/internal/modules/payments/domain"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type OrderReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*ordersdomain.Order, error)
}

type OrderPaymentUpdater interface {
	SetPaymentStatus(ctx context.Context, orderID uuid.UUID, status ordersdomain.PaymentStatus) error
}

type PaymentService struct {
	repo    domain.Repository
	gateway domain.PaymentGateway
	orders  OrderReader
	orderPay OrderPaymentUpdater
	notifier Notifier // may be nil
	audit    AuditLogger // may be nil
}

// Notifier mirrors the pattern in orders/application — see that
// package's Notifier for the full rationale.
type Notifier interface {
	Notify(ctx context.Context, in notificationsdomain.NotifyInput)
}

// AuditLogger mirrors the same pattern, added in Phase 8 — see
// restaurants/application's AuditLogger for the full rationale.
type AuditLogger interface {
	LogAction(ctx context.Context, adminUserID uuid.UUID, action, resourceType string, resourceID *uuid.UUID, details map[string]interface{}, ipAddress *string)
}

func NewPaymentService(repo domain.Repository, gateway domain.PaymentGateway, orders OrderReader, orderPay OrderPaymentUpdater, notifier Notifier, audit AuditLogger) *PaymentService {
	return &PaymentService{repo: repo, gateway: gateway, orders: orders, orderPay: orderPay, notifier: notifier, audit: audit}
}

func (s *PaymentService) notify(ctx context.Context, order *ordersdomain.Order, category notificationsdomain.Category, title, body string) {
	if s.notifier == nil {
		return
	}
	s.notifier.Notify(ctx, notificationsdomain.NotifyInput{
		UserID: order.CustomerID, Category: category, Title: title, Body: body,
		Data: map[string]interface{}{"order_id": order.ID.String(), "order_number": order.OrderNumber},
	})
}

// Initiate starts a payment for an order: validates the order belongs to
// the customer and is still awaiting payment, opens a gateway order, and
// returns what the client needs to launch the gateway's checkout SDK.
// COD orders should never call this — they're marked paid on delivery
// instead (see docs/assumptions.md).
func (s *PaymentService) Initiate(ctx context.Context, orderID, customerID uuid.UUID, method domain.Method) (*domain.InitiateResult, error) {
	if method == domain.MethodCOD {
		return nil, apperr.New(apperr.CodeValidation, "cash-on-delivery orders do not require online payment initiation")
	}

	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.CustomerID != customerID {
		return nil, apperr.Forbidden("you do not have access to this order")
	}
	if order.PaymentStatus != ordersdomain.PaymentPending {
		return nil, apperr.New(apperr.CodeConflict, fmt.Sprintf("order payment is already %s", order.PaymentStatus))
	}
	if order.Status == ordersdomain.StatusCancelled {
		return nil, apperr.New(apperr.CodeConflict, "cannot pay for a cancelled order")
	}

	gatewayOrderID, err := s.gateway.CreateOrder(ctx, order.TotalAmount, "INR", order.OrderNumber)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeUnavailable, "failed to initiate payment with gateway", err)
	}

	tx, err := s.repo.CreateTransaction(ctx, &domain.Transaction{
		OrderID: orderID, CustomerID: customerID, Amount: order.TotalAmount, Currency: "INR",
		Method: method, Gateway: s.gateway.Name(), GatewayOrderID: &gatewayOrderID,
	})
	if err != nil {
		return nil, err
	}

	return &domain.InitiateResult{
		Transaction: tx, GatewayOrderID: gatewayOrderID, GatewayKey: "public_key_placeholder",
		Amount: order.TotalAmount, Currency: "INR",
	}, nil
}

// Capture verifies the client's callback (or a webhook — same
// verification path either way) and, only on success, marks the
// transaction captured and the order paid. This is the ONLY code path
// allowed to mark an order as paid — never trust a client-reported
// "success" without going through gateway.VerifyCallback first.
func (s *PaymentService) Capture(ctx context.Context, cb domain.CaptureCallback) (*domain.Transaction, error) {
	tx, err := s.repo.GetTransactionByGatewayOrderID(ctx, s.gateway.Name(), cb.GatewayOrderID)
	if err != nil {
		return nil, err
	}
	if tx.Status == domain.TxCaptured {
		return tx, nil // idempotent: webhook + client callback can both arrive for the same payment
	}
	if tx.Status == domain.TxFailed {
		return nil, apperr.New(apperr.CodeConflict, "this payment attempt already failed; initiate a new one")
	}

	if err := s.gateway.VerifyCallback(cb); err != nil {
		_, markErr := s.repo.MarkFailed(ctx, tx.ID, "signature verification failed")
		if markErr != nil {
			return nil, markErr
		}
		if order, orderErr := s.orders.GetByID(ctx, tx.OrderID); orderErr == nil {
			s.notify(ctx, order, notificationsdomain.CategoryPaymentFailed, "Payment failed", fmt.Sprintf("We couldn't verify your payment for order %s. Please try again.", order.OrderNumber))
		}
		return nil, err
	}

	captured, err := s.repo.MarkCaptured(ctx, tx.ID, cb.GatewayPaymentID, cb.GatewaySignature)
	if err != nil {
		return nil, err
	}

	if err := s.orderPay.SetPaymentStatus(ctx, tx.OrderID, ordersdomain.PaymentPaid); err != nil {
		// Payment itself succeeded and is durably recorded — an error
		// updating the order's denormalized payment_status is logged
		// upstream but must not make this call appear to fail, since
		// the money has genuinely moved. A reconciliation job (not yet
		// built) should catch orders whose payment_status lags their
		// captured transaction — flagged in docs/assumptions.md.
		return captured, nil
	}

	if order, orderErr := s.orders.GetByID(ctx, tx.OrderID); orderErr == nil {
		s.notify(ctx, order, notificationsdomain.CategoryPaymentCaptured, "Payment received", fmt.Sprintf("Payment of %.2f received for order %s.", tx.Amount, order.OrderNumber))
	}

	return captured, nil
}

// MarkFailed is called when the gateway reports a failed attempt
// (webhook, or client reports failure) rather than the customer simply
// abandoning checkout (which leaves the transaction in 'initiated' and
// is not itself a failure).
func (s *PaymentService) MarkFailed(ctx context.Context, gatewayOrderID, reason string) (*domain.Transaction, error) {
	tx, err := s.repo.GetTransactionByGatewayOrderID(ctx, s.gateway.Name(), gatewayOrderID)
	if err != nil {
		return nil, err
	}
	return s.repo.MarkFailed(ctx, tx.ID, reason)
}

func (s *PaymentService) GetLatestForOrder(ctx context.Context, orderID uuid.UUID) (*domain.Transaction, error) {
	return s.repo.GetLatestTransactionForOrder(ctx, orderID)
}

func (s *PaymentService) ListForOrder(ctx context.Context, orderID uuid.UUID) ([]*domain.Transaction, error) {
	return s.repo.ListTransactionsByOrder(ctx, orderID)
}

// Refund issues a full or partial refund against an order's captured
// payment. Only allowed against a captured transaction; the requested
// amount plus any prior completed refunds must not exceed the original
// captured amount.
func (s *PaymentService) Refund(ctx context.Context, orderID uuid.UUID, amount float64, reason string, initiatedBy *uuid.UUID) (*domain.Refund, error) {
	tx, err := s.repo.GetLatestTransactionForOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if !tx.IsSettled() {
		return nil, apperr.New(apperr.CodeConflict, "cannot refund a payment that was never captured")
	}

	alreadyRefunded, err := s.repo.SumCompletedRefundsForOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if alreadyRefunded+amount > tx.Amount+0.01 { // small epsilon for float rounding
		return nil, apperr.Validation("refund amount exceeds the remaining refundable balance", map[string]interface{}{
			"captured_amount": tx.Amount, "already_refunded": alreadyRefunded, "requested": amount,
		})
	}

	refund, err := s.repo.CreateRefund(ctx, &domain.Refund{
		PaymentTransactionID: tx.ID, OrderID: orderID, Amount: amount, Reason: reason, InitiatedBy: initiatedBy,
	})
	if err != nil {
		return nil, err
	}

	gatewayPaymentID := ""
	if tx.GatewayPaymentID != nil {
		gatewayPaymentID = *tx.GatewayPaymentID
	}
	gatewayRefundID, err := s.gateway.Refund(ctx, gatewayPaymentID, amount, reason)
	if err != nil {
		failed, markErr := s.repo.MarkRefundFailed(ctx, refund.ID, err.Error())
		if markErr != nil {
			return nil, markErr
		}
		return failed, apperr.Wrap(apperr.CodeUnavailable, "refund failed at the payment gateway", err)
	}

	completed, err := s.repo.MarkRefundCompleted(ctx, refund.ID, gatewayRefundID)
	if err != nil {
		return nil, err
	}

	newTotal := alreadyRefunded + amount
	newStatus := domain.TxPartiallyRefunded
	orderPaymentStatus := ordersdomain.PaymentPaid // still partially paid, no dedicated "partially_refunded" order status exists yet
	if newTotal >= tx.Amount-0.01 {
		newStatus = domain.TxRefunded
		orderPaymentStatus = ordersdomain.PaymentRefunded
	}
	_ = s.repo.SetStatus(ctx, tx.ID, newStatus)
	_ = s.orderPay.SetPaymentStatus(ctx, orderID, orderPaymentStatus)

	if order, orderErr := s.orders.GetByID(ctx, orderID); orderErr == nil {
		s.notify(ctx, order, notificationsdomain.CategoryRefundProcessed, "Refund processed", fmt.Sprintf("A refund of %.2f has been processed for order %s.", amount, order.OrderNumber))
	}

	if s.audit != nil && initiatedBy != nil {
		s.audit.LogAction(ctx, *initiatedBy, "payment.refund", "payment_transaction", &tx.ID,
			map[string]interface{}{"order_id": orderID.String(), "amount": amount, "reason": reason}, nil)
	}

	return completed, nil
}

func (s *PaymentService) ListRefunds(ctx context.Context, orderID uuid.UUID) ([]*domain.Refund, error) {
	return s.repo.ListRefundsByOrder(ctx, orderID)
}

func (s *PaymentService) SaveMethod(ctx context.Context, customerID uuid.UUID, method domain.Method, gateway, token, label string, makeDefault bool) (*domain.SavedMethod, error) {
	if makeDefault {
		if err := s.repo.UnsetDefaultMethods(ctx, customerID); err != nil {
			return nil, err
		}
	}
	return s.repo.CreateSavedMethod(ctx, &domain.SavedMethod{
		CustomerID: customerID, Method: method, Gateway: gateway, GatewayToken: token, DisplayLabel: label, IsDefault: makeDefault,
	})
}

func (s *PaymentService) ListSavedMethods(ctx context.Context, customerID uuid.UUID) ([]*domain.SavedMethod, error) {
	return s.repo.ListSavedMethods(ctx, customerID)
}

func (s *PaymentService) DeleteSavedMethod(ctx context.Context, id, customerID uuid.UUID) error {
	return s.repo.DeleteSavedMethod(ctx, id, customerID)
}
