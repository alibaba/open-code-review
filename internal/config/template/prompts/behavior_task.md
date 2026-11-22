# Comparison

{{comparison_manifest}}

# Background and provenance

{{background_context}}

# Visible change inventory and role scope

{{change_inventory}}

# Available captured context

{{context_index}}

# Task

Explain the demonstrated problem, mechanism, before/after behavior, affected workflows, compatibility consequences, and verification limits for this comparison. Treat stated intent as a claim until supported. Inspect the complete visible change inventory before deciding the scope. Follow important effects beyond changed files.

Use the investigation protocol, but skip inapplicable paths. Preserve the difference between a required fix, the consequences of the chosen implementation, incidental changes, and pre-existing behavior. Resolve questions answerable from the available code instead of handing them back as generic maintainer decisions.

Deliver a concise maintainer brief: conclusion first, each material effect once, decisive conditions and evidence retained. Explain the practical value and compatibility cost together. When at least two concrete scenarios have evidence-supported before/after behavior, default to a compact before/after table after the conclusion; use Scenario | Before | After columns and localize the labels. For a single change or unverified old behavior, use prose instead of inventing rows. Use ## for sections and ### for subsections, not standalone bold text. Group verification evidence and unresolved limits in one note. End with supported recommendations under a localized `## Pre-merge maintainer decisions`: a required fix, an acceptance check with an observable pass condition, or a genuine choice. Omit the section if none applies. Do not mirror the full investigation or fill every possible section.
