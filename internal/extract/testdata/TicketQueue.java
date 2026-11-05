package app.queue;

import app.billing.Invoice;

public class TicketQueue {
    static final int MAX_ACTIVE_TICKETS = 1;

    public void enqueue(String ticket) {
        items.add(ticket);
    }
}
