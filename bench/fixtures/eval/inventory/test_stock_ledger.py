from stock_ledger import StockLedger


def test_reserve_then_release():
    ledger = StockLedger()
    assert ledger.reserve_stock("sku-1", 2)
    ledger.release_stock("sku-1")
