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
		"order.created", 
		false,           
		false,          
		false,        
		false,       
		nil,            
	)
	if err != nil {
		return fmt.Errorf("failed to declare a queue: %w", err)
	}

	data := map[string]interface{}{
		"productId": productId,
		"quantity":  quantity,
	}


	event := map[string]interface{}{
		"pattern": "order.created", 
		"data":    data,
	}
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	return p.channel.Publish(
		"",   
		q.Name, 
		false,  
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})
}


type OrderService struct {
	repo              repository.IOrderRepository 
	cache             repository.IOrderCache      
	publisher         IPublisher                  
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

	product, err := s.fetchProductInfo(req.ProductID)
	if err != nil {
		log.Printf("Error fetching product %s: %v", req.ProductID, err)
		return nil, errors.New("product not found or service unavailable")
	}


	if product.Qty < req.Quantity {
		return nil, errors.New("insufficient stock")
	}

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


	if err := s.publisher.PublishOrderCreated(order.ProductID, order.Quantity); err != nil {
		log.Printf("Failed to publish order.created event: %v", err)
	} else {
		log.Printf("Published order.created event for product %s", order.ProductID)
	}

	return order, nil
}

func (s *OrderService) GetOrdersByProductID(productID string) ([]repository.Order, error) {
	cacheKey := s.cache.GetCacheKeyForProduct(productID)


	cachedOrders, err := s.cache.Get(cacheKey)
	if err != nil {
		log.Printf("Redis error on get: %v", err) 
	if cachedOrders != nil {
		log.Println("Returning cached orders")
		return cachedOrders, nil
	}

	log.Println("Fetching orders from DB")
	orders, err := s.repo.GetByProductID(productID)
	if err != nil {
		return nil, err
	}

	if err := s.cache.Set(cacheKey, orders); err != nil {
		log.Printf("Redis error on set: %v", err)
	}

	return orders, nil
}