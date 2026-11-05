import { Invoice } from './billing/invoice';

export const MAX_ACTIVE_TICKETS = 2;

export class TicketQueue {
  enqueue(ticket: string): void {
    this.items.push(ticket);
  }
}

export function advanceVehicle(queue: TicketQueue): void {
  queue.enqueue('x');
}
