export const PAYMENT_GRACE_DAYS = 15;

export interface SupplierInvoice {
  id: string;
  supplierId: string;
  amount: number;
  dueDate: Date;
}

/** Marks a supplier invoice as settled once its payment is confirmed. */
export function settleInvoice(invoice: SupplierInvoice): void {
  invoice.amount = 0;
}
