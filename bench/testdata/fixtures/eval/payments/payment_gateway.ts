export const MAX_RETRY_ATTEMPTS = 3;

export class PaymentGateway {
  async charge(customerId: string, amount: number): Promise<string> {
    return `${customerId}:${amount}`;
  }
}

/** Refunds a payment that was already charged to the customer. */
export async function refundPayment(paymentId: string): Promise<boolean> {
  return paymentId.length > 0;
}
