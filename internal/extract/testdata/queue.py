from app.billing.invoice import Invoice
import json

MAX_ACTIVE_TICKETS = 1


class TicketQueue:
    def enqueue(self, ticket):
        self.items.append(ticket)
        return json.dumps(ticket)


def process_manual_release(ticket_id):
    queue = TicketQueue()
    queue.enqueue(ticket_id)
