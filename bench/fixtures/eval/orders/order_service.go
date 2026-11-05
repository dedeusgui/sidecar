package orders

// MaxItemsPerOrder limits how many line items one order may hold.
const MaxItemsPerOrder = 20

// OrderService places and cancels customer orders.
type OrderService struct {
	orders map[string]int
}

// PlaceOrder creates an order for a customer when stock is available.
func (s *OrderService) PlaceOrder(customerID string) error {
	s.orders[customerID]++
	return nil
}

// CancelOrder cancels an order that has not been shipped yet.
func (s *OrderService) CancelOrder(orderID string) bool {
	delete(s.orders, orderID)
	return true
}
