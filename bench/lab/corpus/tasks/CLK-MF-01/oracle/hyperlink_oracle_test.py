import io

import click
from click.formatting import wrap_text

URL = "https://example.com/docs"
OPEN = "\x1b]8;;" + URL + "\x1b\\"
CLOSE = "\x1b]8;;\x1b\\"


def test_style_link_wraps_the_styled_text():
    assert click.style("docs", fg="blue", link=URL) == (
        OPEN + click.style("docs", fg="blue") + CLOSE
    )


def test_style_without_link_is_unchanged():
    assert click.style("docs", fg="blue") == "\x1b[34mdocs\x1b[0m"


def test_unstyle_removes_hyperlink_and_keeps_text():
    assert click.unstyle(click.style("docs", fg="blue", link=URL)) == "docs"


def test_unstyle_still_removes_colors_and_bel_terminated_links():
    assert click.unstyle("\x1b[31mred\x1b[0m") == "red"
    assert click.unstyle("\x1b]8;;" + URL + "\x07link\x1b]8;;\x07") == "link"


def test_echo_without_color_drops_hyperlink():
    buf = io.StringIO()
    click.echo(click.style("docs", link=URL), file=buf, color=False)
    assert buf.getvalue() == "docs\n"


def test_echo_with_color_keeps_hyperlink():
    buf = io.StringIO()
    click.echo(click.style("docs", link=URL), file=buf, color=True)
    assert buf.getvalue() == click.style("docs", link=URL) + "\n"


def test_wrap_counts_only_visible_characters():
    styled = click.style("aaa bbb", link=URL)
    assert wrap_text(styled, width=7) == styled
