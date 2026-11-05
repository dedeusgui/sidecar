STOCK_RESERVE_MINUTES = 30


class StockLedger:
    """Keeps reserved quantities per product and releases expired holds."""

    def __init__(self):
        self.reserved = {}

    def reserve_stock(self, sku, qty):
        self.reserved[sku] = self.reserved.get(sku, 0) + qty
        return True

    def release_stock(self, sku):
        """Give back the reserved units of a product whose hold expired."""
        self.reserved.pop(sku, None)
