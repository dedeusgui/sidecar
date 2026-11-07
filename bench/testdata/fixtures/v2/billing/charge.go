package billing

import "errors"

// ChargeInvoice charges the invoice once; the caller retries on transient errors.
func ChargeInvoice(invoiceID string, cents int) error {
	if cents <= 0 {
		return errors.New("valor invalido")
	}
	return gateway(invoiceID, cents)
}

func gateway(invoiceID string, cents int) error {
	_ = invoiceID
	_ = cents
	return nil
}
