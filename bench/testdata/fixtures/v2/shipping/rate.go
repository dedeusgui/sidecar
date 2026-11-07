package shipping

// CalculateRate returns the freight in cents for a parcel of the given weight in grams.
func CalculateRate(weightGrams int) int {
	if weightGrams <= 1000 {
		return 1500
	}
	return 1500 + (weightGrams-1000)/100*50
}
