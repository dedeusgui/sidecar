package billing

// LateFeePercent is the percentage added to an overdue invoice.
const LateFeePercent = 2

// LateFee returns the fee in cents for an invoice of the given amount.
func LateFee(cents int) int {
	return cents * LateFeePercent / 100
}
