import { settleInvoice } from './supplier_invoice';

test('settleInvoice clears the amount', () => {
  const invoice = { id: '1', supplierId: 's', amount: 10, dueDate: new Date() };
  settleInvoice(invoice);
  expect(invoice.amount).toBe(0);
});
