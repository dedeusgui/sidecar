import click
from click.formatting import wrap_text
from click.testing import CliRunner


def test_wrap_joins_continuation_lines_with_single_space():
    out = wrap_text(
        "Copies every file that changed\n      since the last run.",
        width=80,
        preserve_paragraphs=True,
    )
    assert out == "Copies every file that changed since the last run."


def test_wrap_paragraph_keeps_indent_and_single_spaces():
    out = wrap_text(
        "Copies every file\n    since the last run",
        width=24,
        initial_indent="> ",
        subsequent_indent="  ",
        preserve_paragraphs=True,
    )
    assert out.split() == [">", "Copies", "every", "file", "since", "the", "last", "run"]
    for line in out.splitlines():
        assert "  " not in line.lstrip()


def test_wrap_preformatted_block_is_kept_verbatim():
    out = wrap_text(
        "\b\nkeep   this   spacing\n  more   here",
        width=80,
        preserve_paragraphs=True,
    )
    assert out == "keep   this   spacing\n  more   here"


def test_help_paragraph_renders_with_single_spaces():
    @click.command(
        help="Sync the files.\n\nCopies every file that changed\n      since the last run."
    )
    def cmd():
        pass

    result = CliRunner().invoke(cmd, ["--help"])
    assert result.exit_code == 0
    assert "Copies every file that changed since the last run." in result.output
