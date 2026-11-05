package supplier

// SupplierRegistry stores the suppliers that may send invoices.
type SupplierRegistry struct {
	taxIDs map[string]bool
}

// RegisterSupplier adds a supplier identified by its tax id.
func (r *SupplierRegistry) RegisterSupplier(taxID string) error {
	r.taxIDs[taxID] = true
	return nil
}
