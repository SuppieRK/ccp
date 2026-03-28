# Agent Rule - Site

Use this rule when editing the GitHub Pages site under `site/`, including `index.html`, `styles.css`, `script.js`,
favicons, and site assets.

The goal is not maximal decoration. The goal is a coherent, readable visit card with one visible rhythm.

## Spacing Principle

- Prefer one spacing system for the whole page.
- Do not invent one-off widths, paddings, or margins for individual sections unless there is a real layout constraint.
- Keep the page visually calm: fewer spacing values, reused consistently, beats many bespoke exceptions.
- Default to a golden-ratio-inspired scale and reuse it everywhere.

Recommended token set:

```css
:root {
    --space-2xs: 0.618rem;
    --space-xs: 0.786rem;
    --space-sm: 1rem;
    --space-md: 1.272rem;
    --space-lg: 1.618rem;
    --space-xl: 2.618rem;
    --space-2xl: 4.236rem;
}
```

If an existing page already uses equivalent values, normalize toward this scale instead of introducing a second one.

## Width Rules

- Use one shared content shell for all page content.
- Header may be full-bleed, but its internal alignment must still respect the same content width as the rest of the page
  unless the user explicitly wants otherwise.
- Hero, content sections, proof blocks, filter demos, and footer should all visually align to the same width.
- Avoid per-section max-width drift. If a narrower measure is needed for readability, apply it deliberately to text
  blocks only, not to the whole section frame.

Default model:

- one page-width wrapper
- one section-width container inside it
- optional narrower text measure inside a section

## Vertical Rhythm

- Section padding should use a small number of repeated values, typically `--space-2xl` on desktop and a reduced
  equivalent on smaller screens.
- Within a section, stacked elements should use one repeated gap, usually `--space-lg` or `--space-md`.
- Do not mix many near-identical values like `1.3rem`, `1.35rem`, `1.4rem`, and `1.5rem` in the same layer of the page.
- Headings, ledes, support copy, code blocks, and supporting labels should sit on a predictable rhythm.

Preferred pattern:

- section to section: `--space-2xl`
- stacked content inside a section: `--space-lg`
- dense UI rows: `--space-sm` or `--space-xs`

## Component Spacing

- Card padding should come from the same scale as section spacing.
- Code blocks, install snippets, pills, and buttons should use the same internal padding logic across the page.
- Border radius should also come from a small repeated set, usually based on `0.618rem`, `0.786rem`, or `1rem`.
- Keep installation snippets visually simple. Prefer markdown-like code presentation over decorative chrome unless there
  is a strong reason.

## Typography Rhythm

Use a small, explicit font-size token set as well. Do not let headings, labels, code, and body text drift into one-off
sizes.

Recommended token set:

```css
:root {
    --font-ui: 0.786rem;
    --font-body: 1rem;
    --font-body-lg: 1.272rem;
    --font-code: 1rem;
    --font-h3: 1.272rem;
    --font-h2: clamp(1.618rem, 5vw, 2.618rem);
    --font-h1: clamp(2.618rem, 7vw, 4.236rem);
}
```

- Body copy should usually use a line-height near `1.618`.
- Dense UI text, labels, and metadata can use a tighter rhythm near `1.272`.
- Heading spacing should be driven by the shared spacing scale, not arbitrary margin tweaks.
- Avoid repeating explanatory text when the layout itself already communicates the structure.

## Motion And Interaction Spacing

- Hover, sticky-header, and marquee effects should feel quiet.
- Avoid bright flashes, jumpy border transitions, and spacing changes that make the layout shimmer while scrolling.
- If an animated area needs extra room, allocate it with the same spacing tokens used elsewhere.

## Exceptions

- If a logo, icon, proof example, or code sample has intrinsic size constraints, adjust only that asset wrapper, not the
  global spacing system.
- If mobile layout requires to be stacked rather than side-by-side presentation, preserve the same spacing rhythm
  instead of inventing a second mobile-only system.
- If a section must break the rhythm, document the reason in the code change or PR summary.

## Review Checklist

Before finishing a site change, check:

- Is all content aligned to one obvious width?
- Are section paddings reused rather than bespoke?
- Are stack gaps drawn from one small scale?
- Do code blocks and cards share the same internal spacing logic?
- Does the page still feel consistent on mobile?
- Did any new one-off numbers sneak in without a clear reason?
