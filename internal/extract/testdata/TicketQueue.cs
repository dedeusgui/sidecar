using System.Collections.Generic;

namespace App.Queue
{
    public class TicketQueue
    {
        public const int MaxActiveTickets = 1;

        public void Enqueue(string ticket)
        {
            items.Add(ticket);
        }
    }
}
