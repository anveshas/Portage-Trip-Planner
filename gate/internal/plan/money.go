package plan

// Hotel GST follows the room's nightly tariff: 12% up to ₹7,500, 18% above that.
func hotelTaxRate(nightly int) int {
	if nightly > 7500 {
		return 18
	}
	return 12
}

// roundHalfUp divides (amount * percent) / 100 and rounds halves away from zero.
func roundHalfUp(amount, percent int) int {
	if amount <= 0 || percent <= 0 {
		return 0
	}
	return (amount*percent + 50) / 100
}
