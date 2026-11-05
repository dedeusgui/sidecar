import { refundPayment } from './payment_gateway';

test('refundPayment rejects an empty id', async () => {
  expect(await refundPayment('')).toBe(false);
});
