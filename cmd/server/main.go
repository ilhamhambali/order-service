package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"order-service/internal/handler"
	"order-service/internal/repository"
	"order-service/internal/service"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/streadway/amqp"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// --- Database Connection ---
	dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DATABASE_HOST"),
		os.Getenv("DATABASE_USER"),
		os.Getenv("DATABASE_PASSWORD"),
		os.Getenv("DATABASE_NAME"),
		os.Getenv("DATABASE_PORT"),
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	db.AutoMigrate(&repository.Order{})

	redisAddr := fmt.Sprintf("%s:%s", os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT"))
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	if _, err := rdb.Ping(context.Background()).Result(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	rabbitURL := os.Getenv("RABBITMQ_URL")
	conn, err := amqp.Dial(rabbitURL)
	if err != nil {
		log.Fatalf("Failed to connect to RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("Failed to open a channel: %v", err)
	}
	defer ch.Close()

	productServiceURL := os.Getenv("PRODUCT_SERVICE_URL")
	repo := repository.NewOrderRepository(db)
	cache := repository.NewOrderCache(rdb)
	publisher := service.NewRabbitMQPublisher(ch)
	orderService := service.NewOrderService(repo, cache, publisher, productServiceURL)
	orderHandler := handler.NewOrderHandler(orderService)

	router := gin.Default()
	router.POST("/orders", orderHandler.CreateOrder)
	router.GET("/orders/product/:productId", orderHandler.GetOrdersByProductID)

	log.Println("Order service is running on :8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}