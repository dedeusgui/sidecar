import click
from click.shell_completion import BashComplete


@click.command()
@click.option("--level", type=click.Choice(["-1", "-2", "x"]))
@click.option("--note", multiple=True, type=click.Choice(["-a", "b"]))
def cli(level, note):
    pass


def _complete(args, incomplete):
    comp = BashComplete(cli, {}, "cli", "_CLI_COMPLETE")
    return [item.value for item in comp.get_completions(args, incomplete)]


def test_attached_dash_value_completes_choice_values():
    assert _complete([], "--level=-") == ["-1", "-2"]


def test_attached_dash_value_is_narrowed_by_prefix():
    assert _complete([], "--level=-1") == ["-1"]


def test_attached_plain_value_completes_choice_values():
    assert _complete([], "--level=x") == ["x"]


def test_attached_empty_value_offers_all_choices():
    assert _complete([], "--level=") == ["-1", "-2", "x"]


def test_attached_dash_value_for_multiple_option():
    assert _complete([], "--note=-") == ["-a"]


def test_attached_plain_value_for_multiple_option():
    assert _complete([], "--note=b") == ["b"]


def test_option_name_prefix_still_completes_names():
    assert _complete([], "--le") == ["--level"]


def test_lone_dash_still_lists_option_names():
    names = _complete(["--level"], "-")
    assert all(name.startswith("-") for name in names)
    assert "--note" in names
    assert "--help" in names


def test_value_after_separate_option_still_offers_choices():
    assert _complete(["--level"], "") == ["-1", "-2", "x"]
