namespace Shop.Receipts
{
    public class ReceiptPrinter
    {
        public const int TaxRatePercent = 17;

        public string PrintInvoice(decimal total)
        {
            return $"TOTAL {total}";
        }
    }
}
