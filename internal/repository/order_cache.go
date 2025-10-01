package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type IOrderCache interface {
	Get(key string) ([]Order, error)
	Set(key string, orders []Order) error
	GetCacheKeyForProduct(productID string) string
}

type OrderCache struct {
	client *redis.Client
	ctx    context.Context
}

var _ IOrderCache = &OrderCache{}

func NewOrderCache(client *redis.Client) *OrderCache {
	return &OrderCache{
		client: client,
		ctx:    context.Background(),
	}
}

func (c *OrderCache) Get(key string) ([]Order, error) {
	val, err := c.client.Get(c.ctx, key).Result()
	if err == redis.Nil {
		return nil, nil 
	} else if err != nil {
		return nil, err
	}

	var orders []Order
	err = json.Unmarshal([]byte(val), &orders)
	return orders, err
}

func (c *OrderCache) Set(key string, orders []Order) error {
	val, err := json.Marshal(orders)
	if err != nil {
		return err
	}
	return c.client.Set(c.ctx, key, val, 60*time.Second).Err()
}

func (c *OrderCache) GetCacheKeyForProduct(productID string) string {
	return fmt.Sprintf("orders:product:%s", productID)
}