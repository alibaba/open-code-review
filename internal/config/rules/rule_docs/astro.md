#### Obvious Typos or Spelling Errors
- Spelling errors in component names, props, slots, or user-facing strings that affect readability

#### Dead Code
- Unused islands, framework components, scripts, or template branches that add client cost without affecting rendered behavior

#### Astro Component Boundaries
- When frontmatter data reaches client HTML, inline scripts, or hydrated islands, verify whether it was computed at build time or request time and whether exposing non-`PUBLIC_` env values, cookies, headers, sessions, `Astro.locals`, secrets, request-only data, or server-only APIs is intentional
- Flag `.astro` templates that appear to assume frontmatter values are reactive in the browser
- Flag framework components used only to render static markup when plain Astro markup would avoid unnecessary client JavaScript

#### Hydration and Islands
- `client:*` applies only to directly imported UI framework components, not `.astro` components or dynamic tags
- Flag `client:load` on non-critical UI, missed `client:idle` or `client:visible` opportunities, `client:media` where the media query does not actually gate the interaction need, and over-hydration from large or overly numerous islands
- Flag `client:only` without the framework string or without fallback content when the result is blank or confusing pre-hydration UI

#### Server-to-Client Data Transfer
- Flag hydrated framework component props or server-fetched data passed client-side without reducing to the minimal interaction payload; props crossing hydrated boundaries must use Astro-supported serializable types, so flag functions, class instances, circular objects, secrets, and unnecessarily large payloads.
- `<script define:vars>` values are JSON-stringified and inline; flag secrets, large payloads, or repeated per-instance duplication

#### Server Islands
- Flag `server:defer` usage without required adapter support or without a fallback slot when deferred content needs a meaningful loading state
- Props passed into `server:defer` islands must use Astro-supported serializable types; flag functions, circular objects, secrets, and large request objects
- Flag `server:defer` uses that leak request-specific data into cacheable output or weaken privacy assumptions

#### Template Safety
- Treat `set:html` as a high-risk escape hatch; flag it unless the source is clearly trusted or sanitized
- Flag unsafe or insufficiently validated dynamic values inserted into attributes, URLs, or raw markup
- Flag fragile template structures, especially mixed `set:*` usage or conditional markup that changes HTML shape in surprising ways

#### Scripts
- Flag scripts with attributes other than `src` when unprocessed behavior causes avoidable per-instance duplication, bypasses bundling, or relies on server-only values
- Flag framework hydration used for behavior that a small processed Astro script would handle

#### Styles
- Flag unnecessary `is:global` usage when scoped styles or a narrow `:global(...)` escape would do
- Flag selectors that assume scoped CSS can style child component internals across a component boundary
- Flag components that accept parent styling but fail to forward `class` and needed rest props

#### Content and Assets
- For structured Markdown/MDX/JSON content, flag ad hoc loading when Astro content collections would materially improve schema validation, typing, or route generation
- Flag plain `<img>` or `public/` asset usage when the implementation appears to expect Astro image optimization, responsive behavior, fingerprinting, transforms, or import-time validation

#### Markup and Accessibility
- Flag non-semantic or fragile interactive markup, including inaccessible islands before hydration, invalid conditional HTML, or mixed Astro/framework composition that breaks keyboard or focus behavior
