> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat factual errors and placeholder mismatches as blocking, and style suggestions as non-blocking.

#### Factual Errors in Translation
- The `msgstr` contradicts or distorts the meaning of its `msgid` (mistranslation, omitted clauses, or text belonging to a different entry)
- Numbers, units, dates, or proper nouns in the `msgstr` that do not match the `msgid` (e.g., "100 MB" translated as "100 GB")
- Do not report subjective wording preferences, tone, or regional variants when the meaning is preserved

#### Format and Structure
- Unbalanced or unescaped quotes in `msgid`/`msgstr` strings, breaking the entry
- Multi-line continuation strings concatenated incorrectly (missing trailing space/newline between fragments that changes the resulting text)
- `msgstr` missing entirely for a non-fuzzy entry, or orphaned `msgstr` without a preceding `msgid`
- Duplicate `msgid` definitions within the file that conflict with each other

#### Placeholder Mismatch
- Format placeholders (`%s`, `%d`, `%.2f`, `%(name)s`) present in the `msgid` but missing, reordered (without positional markers like `%1$s`), or changed in type in the `msgstr`
- Named placeholders renamed in the `msgstr` (e.g., `%(user)s` becoming `%(name)s`), which breaks lookups at runtime
- Brace-style placeholders (`{0}`, `{name}`, `{{count}}`) whose count or names differ between `msgid` and `msgstr`
- Do not report reordering that is correctly expressed with explicit positional markers

#### Plural Forms
- Number of `msgstr[n]` entries does not match the `nplurals` declared in the `Plural-Forms` header
- `msgid_plural` present but only `msgstr[0]` provided, or `msgstr[n]` indices that skip values
- A language whose plural rules require multiple forms (e.g., Arabic, Russian, Polish) given a single form that copies the singular, when the count varies

#### Escapes and Surrounding Whitespace
- Broken escape sequences (`\n`, `\t`, `\"`) that render literally or terminate the string early
- Leading/trailing whitespace or trailing `\n` present in the `msgid` but missing (or added) in the `msgstr`, causing layout or concatenation differences
- Encoding-corrupted characters (mojibake) in the `msgstr`

