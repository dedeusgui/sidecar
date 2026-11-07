package orders

import "example.com/v2/inventory"

// PlaceOrder creates an order and reserves its stock.
func PlaceOrder(sku string, qty int) error {
	return inventory.Reserve(sku, qty)
}
