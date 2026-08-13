# Hashline anchor comment localization (experimental)

Renders the review diff with per-line anchors (`LINE#HASH:`) and lets the
model localize `code_comment` calls by copying an anchor instead of quoting
`existing_code`. The anchor's hash is verified against the new file content
(two-factor: line number = primary key, hash = checksum, existing_code =
text hint), eliminating the first-match ambiguity of pure text matching.

Adapted from the hashline protocol (github.com/RimuruW/pi-hashline-edit).

## Usage

```bash
OCR_HASHLINE_ANCHORS=1 opencodereview review \
  --from <base> --to <head> \
  --tools examples/hashline_anchors/tools.json
```

- `OCR_HASHLINE_ANCHORS=1` — annotate the diff shown to the model with anchors.
- `--tools examples/hashline_anchors/tools.json` — code_comment schema with the
  `anchor` parameter and matching description.

Comments resolved via a verified anchor report `loc_method: "anchor"` in JSON
output; a hash mismatch falls back to the existing text-matching pipeline
(`hunk` / `file` / `relocation`), so behavior is never worse than baseline.

## Measured effect (offline replay over real commits, production resolver)

| Localization | opencode repo (95k added lines) | this repo (23k added lines) |
|---|---|---|
| existing_code, 1 line | 63.2% correct | 74.4% correct |
| existing_code, 3 lines | 81.8% correct | 93.0% correct |
| hashline anchor | 100% correct | 100% correct |

Anchor false-accept rate (wrong line number still passing hash verification):
~0.5%. Diff token overhead of annotation: +26% on the diff itself; in
end-to-end runs total input tokens dropped ~25% as the model needed fewer
file_read round-trips to confirm positions.
