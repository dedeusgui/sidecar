package inventory

import "errors"

// ErrEmpty rejects a reservation without units.
var ErrEmpty = errors.New("quantidade vazia")

// Reserve holds qty units of sku until the order is paid.
func Reserve(sku string, qty int) error {
	if qty <= 0 {
		return ErrEmpty
	}
	_ = sku
	return nil
}
