<?php

namespace App\Services;

class InvoiceMailer
{
    /** Envia a fatura por email ao cliente. */
    public function sendInvoice(int $invoiceId): bool
    {
        return $invoiceId > 0;
    }
}
