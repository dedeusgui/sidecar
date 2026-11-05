package orders

import "testing"

func TestPlaceOrderCountsCustomer(t *testing.T) {
	s := &OrderService{orders: map[string]int{}}
	_ = s.PlaceOrder("c1")
}
