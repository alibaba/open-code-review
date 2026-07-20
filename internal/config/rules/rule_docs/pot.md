> Favor precision over recall: only raise an issue when you are confident it is a real defect, and stay silent when the surrounding context is unclear — a false alarm costs more reviewer trust than a missed minor issue. Treat structural errors and placeholder mismatches as blocking, and style suggestions as non-blocking. In a template (.pot) file every `msgstr` is expected to be empty; do not report empty `msgstr` entries as missing translations.

#### Header Integrity
- Missing or malformed `Content-Type` header, or a charset that does not match the file's actual encoding
- `Plural-Forms` header with a syntactically invalid `nplurals`/`plural` expression, or one that does not parse as a C-style ternary expression
- Do not report missing optional metadata fields (e.g., `Project-Id-Version`, `Report-Msgid-Bugs-To`)

#### Format and Structure
- Unbalanced or unescaped quotes in `msgid`/`msgid_plural` strings, breaking the entry
- Multi-line continuation strings concatenated incorrectly (missing trailing space/newline between fragments that changes the resulting text)
- Orphaned `msgid_plural` or `msgstr` without a preceding `msgid`
- Duplicate `msgid` definitions within the file that conflict with each other (different `msgctxt`, comments, or placeholders)
- A non-empty `msgstr` in a template entry, which usually means a translation was accidentally committed into the template

#### Placeholder Consistency
- Format placeholders (`%s`, `%d`, `%.2f`, `%(name)s`) present in the `msgid` but missing, reordered (without positional markers like `%1$s`), or changed in type in the `msgid_plural`
- Named placeholders renamed between `msgid` and `msgid_plural` (e.g., `%(user)s` becoming `%(name)s`), which breaks lookups at runtime
- Brace-style placeholders (`{0}`, `{name}`, `{{count}}`) whose count or names differ between `msgid` and `msgid_plural`
- Do not report reordering that is correctly expressed with explicit positional markers

#### Plural Forms
- `msgid_plural` present but no `Plural-Forms` header declared, or a `Plural-Forms` header whose `nplurals` is inconsistent with the `plural` expression's reachable form count
- Singular-only entries (`msgid` without `msgid_plural`) whose text embeds a count placeholder (e.g., `%d files`), indicating a plural form was forgotten
- A `plural` expression that is constant (always evaluates to the same index), defeating the purpose of plural selection

#### Escapes and Surrounding Whitespace
- Broken escape sequences (`\n`, `\t`, `\"`) that render literally or terminate the string early
- Leading/trailing whitespace or trailing `\n` that differs between `msgid` and `msgid_plural` in a way that changes layout or concatenation
- Encoding-corrupted characters (mojibake) in any string
