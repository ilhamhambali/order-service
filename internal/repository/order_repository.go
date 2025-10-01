package repository

import (
	"time"
	"gorm.io/gorm"
)
type IOrderRepository interface {
	Create(order *Order) error
	GetByProductID(productID string) ([]Order, error)
}
type Order struct {
	ID         string    `gorm:"type:uuid;primary_key;"`
	ProductID  string    `gorm:"not null"`
	TotalPrice float64   `gorm:"not null"`
	Quantity   int       `gorm:"not null"`
	Status     string    `gorm:"not null"`
	CreatedAt  time.Time
}

type OrderRepository struct { db *gorm.DB }

var _ IOrderRepository = &OrderRepository{}

func NewOrderRepository(db *gorm.DB) *OrderRepository { return &OrderRepository{db: db} }
func (r *OrderRepository) Create(order *Order) error { return r.db.Create(order).Error }
func (r *OrderRepository) GetByProductID(productID string) ([]Order, error) {
	var orders []Order
	err := r.db.Where("product_id = ?", productID).Find(&orders).Error
	return orders, err
}