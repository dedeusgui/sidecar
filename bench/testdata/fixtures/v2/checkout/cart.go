package checkout

import "example.com/v2/shipping"

// Checkout totals the cart and adds the shipping computed by the rate table.
func Checkout(total, weightGrams int) int {
	return total + shipping.CalculateRate(weightGrams)
}
