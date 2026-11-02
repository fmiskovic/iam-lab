package main

import (
	"errors"
	"fmt"
	"sync"
)

var (
	errOrderNotFound = errors.New("order not found")
	errNotOrderOwner = errors.New("order belongs to another user")
	errOrderPaid     = errors.New("order is already paid")
)

type order struct {
	ID          string
	Owner       string
	AmountCents int64
	Currency    string
	Paid        bool
}

type payment struct {
	ID       string `json:"payment_id"`
	OrderID  string `json:"order_id"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// orderBook is payment-api's view of the shop's orders, kept in memory for the lab.
type orderBook struct {
	mu     sync.Mutex
	orders map[string]order
}

func newOrderBook() *orderBook {
	return &orderBook{orders: map[string]order{
		"42": {ID: "42", Owner: "alice", AmountCents: 12990, Currency: "EUR"},
		"43": {ID: "43", Owner: "bob", AmountCents: 5400, Currency: "EUR"},
	}}
}

// pay marks the caller's order as paid. The amount comes from the order, never from the request.
func (b *orderBook) pay(orderID, username string) (payment, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	current, ok := b.orders[orderID]
	if !ok {
		return payment{}, errOrderNotFound
	}
	// Lab shortcut: ownership by username keeps the seed data readable. Real services
	// key it on sub, which is stable and unique per issuer; usernames can change.
	if current.Owner != username {
		return payment{}, errNotOrderOwner
	}
	if current.Paid {
		return payment{}, errOrderPaid
	}

	paid := current
	paid.Paid = true
	b.orders[orderID] = paid

	return payment{
		ID:       "pay-" + orderID,
		OrderID:  orderID,
		Amount:   fmt.Sprintf("%d.%02d", paid.AmountCents/100, paid.AmountCents%100),
		Currency: paid.Currency,
	}, nil
}
