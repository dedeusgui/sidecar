import click


def _values(choice, incomplete):
    return [item.value for item in choice.shell_complete(None, None, incomplete)]


def test_ci_completion_offers_declared_spelling():
    c = click.Choice(["Apple", "banana"], case_sensitive=False)
    assert _values(c, "A") == ["Apple"]


def test_ci_completion_matches_any_case_and_keeps_declared_order():
    c = click.Choice(["Apple", "banana", "apricot"], case_sensitive=False)
    assert _values(c, "AP") == ["Apple", "apricot"]


def test_ci_completion_empty_lists_all_declared_spellings():
    c = click.Choice(["Apple", "banana"], case_sensitive=False)
    assert _values(c, "") == ["Apple", "banana"]


def test_ci_convert_still_accepts_any_case():
    c = click.Choice(["Apple", "banana"], case_sensitive=False)
    assert c.convert("bAnAnA", None, None) == "banana"
    assert c.convert("APPLE", None, None) == "Apple"


def test_case_sensitive_completion_is_unchanged():
    c = click.Choice(["Apple", "banana"])
    assert _values(c, "A") == ["Apple"]
    assert _values(c, "a") == []
