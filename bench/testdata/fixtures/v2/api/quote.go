package api

import "example.com/v2/shipping"

// QuoteHandler answers the shipping quote request of the storefront.
func QuoteHandler(weightGrams int) int {
	return shipping.CalculateRate(weightGrams)
}
