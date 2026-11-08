import click
import pytest


def test_ci_invalid_message_lists_declared_spellings():
    c = click.Choice(["Apple", "banana"], case_sensitive=False)
    with pytest.raises(click.BadParameter) as exc:
        c.convert("cherry", None, None)
    msg = exc.value.message
    assert "'Apple'" in msg
    assert "'banana'" in msg
    assert "'apple'" not in msg


def test_ci_invalid_message_via_get_invalid_choice_message():
    c = click.Choice(["Apple", "banana"], case_sensitive=False)
    msg = c.get_invalid_choice_message("cherry", None)
    assert msg == "'cherry' is not one of 'Apple', 'banana'."


def test_ci_valid_value_still_converts_to_declared_choice():
    c = click.Choice(["Apple", "banana"], case_sensitive=False)
    assert c.convert("APPLE", None, None) == "Apple"


def test_case_sensitive_invalid_message_is_unchanged():
    c = click.Choice(["apple", "banana"])
    assert c.get_invalid_choice_message("x", None) == "'x' is not one of 'apple', 'banana'."
