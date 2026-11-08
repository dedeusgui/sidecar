import re

import click
from click.testing import CliRunner

PREFIX = "CLK_H1"


def _make(envvar, required=False, help_text="Token."):
    @click.command()
    @click.option(
        "--tok",
        envvar=envvar,
        show_envvar=True,
        required=required,
        help=help_text,
    )
    def cmd(tok):
        click.echo(f"tok={tok}")

    return cmd


def _help_env_names(envvar, required=False):
    out = CliRunner().invoke(_make(envvar, required=required), ["--help"]).output
    match = re.search(r"env var: ([^;\]]*)", out)
    assert match is not None, out
    raw = match.group(1)
    return [] if raw == "" else raw.split(", ")


def _tok_help_line(envvar):
    out = CliRunner().invoke(_make(envvar), ["--help"]).output
    lines = [line for line in out.splitlines() if "--tok" in line]
    assert len(lines) == 1, out
    return lines[0]


def _missing_message(envvar):
    res = CliRunner().invoke(_make(envvar, required=True), [], env={})
    assert res.exit_code == 2, res.output
    return res.output.strip().splitlines()[-1]


def test_help_drops_empty_name_at_start():
    assert _help_env_names(["", f"{PREFIX}_B"]) == [f"{PREFIX}_B"]


def test_help_drops_empty_name_in_the_middle():
    names = [f"{PREFIX}_A", "", f"{PREFIX}_B"]
    assert _help_env_names(names) == [f"{PREFIX}_A", f"{PREFIX}_B"]


def test_help_with_only_empty_names_matches_no_envvar():
    without = _tok_help_line(None)
    assert _tok_help_line(["", ""]) == without
    assert _tok_help_line([""]) == without
    assert _tok_help_line("") == without


def test_help_keeps_names_when_no_entry_is_empty():
    assert _help_env_names([f"{PREFIX}_A", f"{PREFIX}_B"], required=True) == [
        f"{PREFIX}_A",
        f"{PREFIX}_B",
    ]
    out = CliRunner().invoke(_make([f"{PREFIX}_A", f"{PREFIX}_B"], required=True), ["--help"]).output
    assert f"[env var: {PREFIX}_A, {PREFIX}_B; required]" in out


def test_missing_error_names_only_non_empty_variables():
    msg = _missing_message(["", f"{PREFIX}_B"])
    assert msg == f"Error: Missing option '--tok' (env var: '{PREFIX}_B')."


def test_missing_error_keeps_format_with_named_variables():
    msg = _missing_message([f"{PREFIX}_A", f"{PREFIX}_B"])
    assert msg == f"Error: Missing option '--tok' (env var: '{PREFIX}_A', '{PREFIX}_B')."


def test_missing_error_with_only_empty_names_matches_no_envvar():
    assert _missing_message(["", ""]) == _missing_message(None)
    assert _missing_message([""]) == _missing_message(None)
    assert "env var" not in _missing_message([""])


def test_empty_name_does_not_change_value_resolution():
    res = CliRunner().invoke(_make(["", f"{PREFIX}_B"]), [], env={f"{PREFIX}_B": "v"})
    assert res.exit_code == 0, res.output
    assert res.output.strip() == "tok=v"
