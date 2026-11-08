import inspect
import warnings

import click
from click.formatting import wrap_text
from click.testing import CliRunner


def test_keep_paragraphs_keyword_is_accepted():
    out = wrap_text("first line\nsecond line", width=80, keep_paragraphs=True)
    assert out == "first line second line"


def test_keep_paragraphs_matches_preserve_paragraphs_result():
    text = "para one\nstill one\n\npara two"
    new = wrap_text(text, width=12, keep_paragraphs=True)
    old = wrap_text(text, width=12, preserve_paragraphs=True)
    assert new == old == "para one\nstill one\n\npara two"


def test_preserve_paragraphs_still_works_without_warning():
    with warnings.catch_warnings(record=True) as caught:
        warnings.simplefilter("always")
        out = wrap_text("a\nb", width=80, preserve_paragraphs=True)
    assert out == "a b"
    assert not caught


def test_default_still_does_not_keep_paragraphs():
    assert wrap_text("a\nb", width=80) == "a\nb"


def test_keep_paragraphs_parameter_default_is_false():
    param = inspect.signature(wrap_text).parameters["keep_paragraphs"]
    assert param.default is False


def test_help_paragraphs_render_as_before():
    @click.command(help="First paragraph here.\n\nSecond paragraph here.")
    def cmd():
        pass

    result = CliRunner().invoke(cmd, ["--help"])
    assert result.exit_code == 0
    assert "  First paragraph here.\n\n  Second paragraph here.\n" in result.output
