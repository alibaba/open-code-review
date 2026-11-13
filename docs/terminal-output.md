# Terminal output

The CLI separates presentation from result data. Terminal styling does not make
another model request, rewrite findings, or change JSON schemas.

## Markdown summaries

`ocr scan` renders the project summary as Markdown on interactive terminal stdout:
headings, emphasis, inline code, lists, and tables are styled rather than printed
with their Markdown delimiters. Prose wraps to the terminal width without a report
length limit or summarization. Review comments keep their existing layout.

The renderer uses the terminal's default foreground/background and basic ANSI
colors. It does not query the background, load external themes, execute embedded
content, or fetch links and images. Only styling escape sequences are allowed;
terminal commands and active hyperlink sequences are removed. A rendering failure
falls back to sanitized Markdown instead of failing the run.

Markdown follows CommonMark/GFM delimiter rules. For bold labels, prefer putting
punctuation outside the emphasis (`**Label**:text`) or separating the following
text with a space (`**Label:** text`). A closing delimiter immediately between
punctuation and a letter (`**Label:**text`) can remain literal. This is a Markdown
syntax boundary, not a difference between terminal applications.

## Progress styling

Live progress uses a dim `[ocr]` prefix, cyan bold tool names, and colored status
markers: cyan for start, green for success, and red for failure. Arguments, message
text, and timing values are unchanged. `--audience agent` still suppresses live
progress; tool failures retain their existing stderr diagnostics.

## Color and output destinations

- `--color=auto` (default): style terminal streams, but not pipes or files.
  `TERM=dumb` disables automatic styling.
- `--color=never`: disable styling and keep Markdown delimiters.
- `--color=always`: force styling, including redirected console streams, for
  example `ocr scan --color=always | less -R`.
- `--output <file>`: result files are not Markdown-rendered, even with
  `--color=always`. Text output still sanitizes unsafe terminal controls.
- `--format json`: stdout stays a plain JSON document, and `project_summary`
  retains the original Markdown string. With a human audience, progress goes to
  stderr and is styled according to that stream's terminal status, independently
  of stdout. SARIF output also remains unstyled.

For local development, `make build` writes `dist/opencodereview`; it does not
replace an installed `ocr` command.
