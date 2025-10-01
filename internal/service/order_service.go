package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"order-service/internal/repository"
	"time"

	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

// DTOs for external communication
type CreateOrderRequest struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

type ProductResponse struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Price float64 `json:"price,string"` // Handle JSON string for number
	Qty   int     `json:"qty"`
}

type IPublisher interface {
	PublishOrderCreated(productId string, quantity int) error
}

// RabbitMQ Event Publisher
type RabbitMQPublisher struct {
	channel *amqp.Channel
}
var _ IPublisher = &RabbitMQPublisher{}

func NewRabbitMQPublisher(ch *amqp.Channel) *RabbitMQPublisher {
	return &RabbitMQPublisher{channel: ch}
}

func (p *RabbitMQPublisher) PublishOrderCreated(productId string, quantity int) error {
	q, err := p.channel.QueueDeclare(
		"order.created", // name
		false,           // durable
		false,           // delete when unused
		false,           // exclusive
		false,           // no-wait
		nil,             // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare a queue: %w", err)
	}

	event := map[string]interface{}{
		"productId": productId,
		"quantity":  quantity,
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	return p.channel.Publish(
		"",     // exchange
		q.Name, // routing key
		false,  // mandatory
		false,  // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})
}

// OrderService contains the business logic
type OrderService struct {
	repo              repository.IOrderRepository // Ganti dari *repository.OrderRepository
	cache             repository.IOrderCache      // Ganti dari *repository.OrderCache
	publisher         IPublisher                  // Ganti dari *RabbitMQPublisher
	productServiceURL string
}

func NewOrderService(repo repository.IOrderRepository, cache repository.IOrderCache, pub IPublisher, productURL string) *OrderService {
	return &OrderService{
		repo:              repo,
		cache:             cache,
		publisher:         pub,
		productServiceURL: productURL,
	}
}

// fetchProductInfo makes a synchronous HTTP call to the product-service
func (s *OrderService) fetchProductInfo(productID string) (*ProductResponse, error) {
	url := fmt.Sprintf("%s/products/%s", s.productServiceURL, productID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call product service: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("product service returned status: %s", resp.Status)
	}

	var product ProductResponse
	if err := json.NewDecoder(resp.Body).Decode(&product); err != nil {
		return nil, fmt.Errorf("failed to decode product response: %w", err)
	}
	return &product, nil
}

func (s *OrderService) CreateOrder(req CreateOrderRequest) (*repository.Order, error) {
	// 1. Fetch product info from product-service
	product, err := s.fetchProductInfo(req.ProductID)
	if err != nil {
		log.Printf("Error fetching product %s: %v", req.ProductID, err)
		return nil, errors.New("product not found or service unavailable")
	}

	// 2. Validate stock
	if product.Qty < req.Quantity {
		return nil, errors.New("insufficient stock")
	}

	// 3. Create and save order
	order := &repository.Order{
		ID:         uuid.New().String(),
		ProductID:  req.ProductID,
		TotalPrice: product.Price * float64(req.Quantity),
		Quantity:   req.Quantity,
		Status:     "PENDING",
		CreatedAt:  time.Now(),
	}

	if err := s.repo.Create(order); err != nil {
		return nil, err
	}

	// 4. Publish event
	if err := s.publisher.PublishOrderCreated(order.ProductID, order.Quantity); err != nil {
		// In a real system, you'd handle this failure (e.g., retry, log to a dead-letter queue)
		log.Printf("Failed to publish order.created event: %v", err)
	} else {
		log.Printf("Published order.created event for product %s", order.ProductID)
	}

	return order, nil
}

func (s *OrderService) GetOrdersByProductID(productID string) ([]repository.Order, error) {
	cacheKey := s.cache.GetCacheKeyForProduct(productID)

	// Check cache first
	cachedOrders, err := s.cache.Get(cacheKey)
	if err != nil {
		log.Printf("Redis error on get: %v", err) // Log error but proceed to DB
	}
	if cachedOrders != nil {
		log.Println("Returning cached orders")
		return cachedOrders, nil
	}

	// If cache miss, get from DB
	log.Println("Fetching orders from DB")
	orders, err := s.repo.GetByProductID(productID)
	if err != nil {
		return nil, err
	}

	// Save to cache
	if err := s.cache.Set(cacheKey, orders); err != nil {
		log.Printf("Redis error on set: %v", err) // Log error but don't fail the request
	}

	return orders, nil
}