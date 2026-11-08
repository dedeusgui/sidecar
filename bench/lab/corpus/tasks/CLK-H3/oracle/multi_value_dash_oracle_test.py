import click
from click.shell_completion import BashComplete

CHOICES = ["-1", "-2", "x", "y"]


@click.command()
@click.option("--pair", nargs=2, type=click.Choice(CHOICES))
@click.option("--level", type=click.Choice(["-1", "-2", "x"]))
@click.argument("items", nargs=-1, type=click.Choice(["p", "q"]))
def cli(pair, level, items):
    pass


def _complete(args, incomplete):
    comp = BashComplete(cli, {}, "cli", "_CLI_COMPLETE")
    return [item.value for item in comp.get_completions(args, incomplete)]


def test_second_value_after_dash_first_value_completes_choices():
    assert _complete(["--pair", "-1"], "") == CHOICES


def test_second_value_after_dash_first_value_is_filtered_by_prefix():
    assert _complete(["--pair", "-1"], "x") == ["x"]


def test_dash_word_after_dash_first_value_still_lists_option_names():
    names = _complete(["--pair", "-1"], "-")
    assert "--level" in names
    assert "--help" in names


def test_second_value_after_plain_first_value_completes_choices():
    assert _complete(["--pair", "x"], "") == CHOICES


def test_complete_pair_with_dash_values_moves_to_arguments():
    assert _complete(["--pair", "-2", "-1"], "") == ["p", "q"]


def test_single_value_option_with_dash_value_moves_to_arguments():
    assert _complete(["--level", "-1"], "") == ["p", "q"]


def test_dash_second_value_completes_pair_and_moves_to_arguments():
    assert _complete(["--pair", "x", "-1"], "") == ["p", "q"]
