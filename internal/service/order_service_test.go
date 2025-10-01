package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"order-service/internal/repository"
	"testing"
)

// Mock OrderRepository (memenuhi IOrderRepository)
type mockOrderRepository struct{}
func (m *mockOrderRepository) Create(order *repository.Order) error { return nil }
func (m *mockOrderRepository) GetByProductID(productID string) ([]repository.Order, error) { return nil, nil }

// Mock OrderCache (memenuhi IOrderCache)
type mockOrderCache struct{}
func (m *mockOrderCache) Get(key string) ([]repository.Order, error) { return nil, nil }
func (m *mockOrderCache) Set(key string, orders []repository.Order) error { return nil }
func (m *mockOrderCache) GetCacheKeyForProduct(productID string) string { return "key" }

// Mock Publisher (memenuhi IPublisher)
type mockPublisher struct{
	shouldFail bool
}
func (m *mockPublisher) PublishOrderCreated(productId string, quantity int) error {
	if m.shouldFail {
		return errors.New("publish failed")
	}
	return nil
}

func TestCreateOrder(t *testing.T) {
	// ... server httptest tetap sama ...
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/products/valid-product" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"valid-product", "name":"Test", "price":"10.0", "qty":100}`))
		} else if r.URL.Path == "/products/no-stock" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"id":"no-stock", "name":"Test", "price":"10.0", "qty":1}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()


	// Setup service with mocks
	service := NewOrderService(
		&mockOrderRepository{},
		&mockOrderCache{},
		&mockPublisher{},
		server.URL,
	)

	// Test Case 1: Successful order creation
	t.Run("successful order creation", func(t *testing.T) {
		req := CreateOrderRequest{ProductID: "valid-product", Quantity: 5}
		order, err := service.CreateOrder(req)

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if order == nil {
			t.Error("Expected order to be created, got nil")
		}
		if order.TotalPrice != 50.0 {
			t.Errorf("Expected total price to be 50.0, got %f", order.TotalPrice)
		}
	})

	// Test Case 2: Insufficient stock
	t.Run("insufficient stock", func(t *testing.T) {
		req := CreateOrderRequest{ProductID: "no-stock", Quantity: 5}
		_, err := service.CreateOrder(req)

		if err == nil {
			t.Error("Expected an error for insufficient stock, got nil")
		}
		if err.Error() != "insufficient stock" {
			t.Errorf("Expected 'insufficient stock' error, got '%v'", err)
		}
	})
}