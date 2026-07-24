# Cylism Manager Pastel Glass UI Design

**Status:** Draft - awaiting user approval

## Goal

Rebuild the Cylism Manager interface as an operational glass UI that retains dense,
scannable infrastructure workflows. The visual system must support multiple color
palettes without changing product behavior, API contracts, routes, or operations.

The visual reference is the local prototype's `06 Pastel Glass` direction. It combines
the muted warm-white, aqua, mint, and orchid palette with translucent operational
surfaces. It does not reuse the CloudCone component layout.

## Design Principles

1. Operations first: risk, service health, and recent changes appear before passive
   totals.
2. Glass supports hierarchy, not decoration: only navigation, panels, toolbars, and
   dialogs use translucent surfaces. Dense tables retain strong row separation.
3. Status is semantic: healthy, warning, failed, and neutral states keep their meaning
   across every palette.
4. One product structure: palettes change tokens only. They do not replace navigation
   or page anatomy.
5. Fallback remains legible: browsers without `backdrop-filter` receive opaque
   surfaces with the same contrast and spacing.

## Application Shell

### Desktop

- A 224px left rail groups Overview, Servers, Routes, Certificates, Resources, Audit,
  and Data Management.
- A compact translucent top bar contains the current environment indicator, palette
  swatch menu, and account/logout actions.
- The page content area uses a fluid max width and a low-contrast geometric background
  field behind glass panels. The background uses broad color bands rather than floating
  gradient orbs.
- Navigation uses Lucide icons, a visible selected state, and standard tooltips for
  icon-only controls.

### Visual Refinement: Direction 05 Alignment

- The desktop top bar is a full-width, unframed primary navigation strip, not a
  floating rounded panel. It contains the brand and the global Overview,
  Infrastructure, and Records destinations; it uses only a subtle bottom divider
  and stays translucent so it reads as part of the scene rather than an additional
  container.
- The side rail is the contextual secondary navigation for the selected top-level
  destination. It never repeats the complete set of global navigation links.
- The background retains Direction 05's two broad, diagonal quadrilateral bands:
  muted aqua enters from the upper right and mist blue enters from the lower left.
  No floating blobs or gradients are used.
- The default palette is `Reference Glass`, matching the Direction 05 prototype
  exactly: canvas `#eaf1f0`, upper band `#c2e4db`, lower band `#d8e4f7`, and teal
  accent `#22736b`. Other palettes remain optional alternatives.
- Glass panels use lower-opacity white, a brighter one-pixel rim, an inset highlight,
  and stronger backdrop blur/saturation. This reveals the geometric field through
  panels while preserving text and table contrast.
- The side rail remains the primary framed navigation surface. Dense content panels
  stay compact and do not acquire extra nested glass containers.

### Mobile

- The rail becomes a modal drawer launched from the top bar.
- Palette selection remains available from the top bar.
- Tables horizontally scroll inside their own viewport; primary actions never overflow.
- Summary panels stack above tables when narrow.

## Palette System

The palette selector exposes four options. The default is Mint Glass.

| Palette | Canvas and glass tint | Accent | Intended setting |
|---|---|---|---|
| Mint Glass | Warm white, aqua, mint, orchid | muted green | Default daily operations |
| Mist Blue | Cool white and blue-grey | steel blue | Table-heavy daytime work |
| Orchid Glass | Warm grey with muted violet | plum | Personal preference, same semantics |
| Night Glass | Graphite with deep teal glass | aqua green | Low-light monitoring |

Each palette defines semantic tokens rather than directly styling components:

- Surface: `canvas`, `glass`, `glass-raised`, `glass-subtle`, `input`, `table-row`,
  `table-row-hover`, `overlay`.
- Content: `text-primary`, `text-secondary`, `text-muted`, `icon`, `focus-ring`.
- Actions: `action-primary`, `action-primary-hover`, `action-secondary`.
- States: `status-success`, `status-success-surface`, `status-warning`,
  `status-warning-surface`, `status-danger`, `status-danger-surface`, `status-neutral`.
- Effects: `glass-border`, `glass-shadow`, `glass-blur`, `page-band-a`,
  `page-band-b`.

No component may directly use raw palette values. Status colors remain fixed by role;
only their compatible foreground/background pairs vary per palette.

## Components

### Glass panels

- Use a 14px radius, a semi-transparent surface, a 1px translucent border, and a
  restrained shadow.
- Do not nest glass cards. A page section is one panel; repeated items use rows inside
  it.
- Tables use a panel wrapper and separated rows, not a card per row.

### Controls

- Primary commands use compact icon plus text only when a clear action label is needed.
- Utility controls such as refresh, palette, and collapse use Lucide icon buttons with
  tooltips.
- Palette switching opens a menu of four labeled color swatches; it is not a binary
  light/dark toggle.
- Focus rings use the palette's semantic focus token and meet visible keyboard focus
  requirements.

### Modals and forms

- The overlay has a higher blur and a darker neutral wash.
- Dialogs are raised glass surfaces with a fixed title/action region.
- Inputs remain more opaque than surrounding glass so values, placeholders, and errors
  stay readable.

## Page Designs

### Login

The login card becomes a compact raised glass form centered on a restrained branded
background. It retains username/password fields, validation feedback, and submit
behavior.

### Dashboard

The first viewport contains a service-health summary, expiring-certificate alert, and
recent operations. K3s state and server count become secondary compact metrics.
Certificate risk remains visually more prominent than neutral totals.

### Servers, Routes, Certificates, Resources, Audit, and Data Management

Each page uses the same header composition: title and contextual primary action,
followed by a glass filter/tool strip and one structured data panel. Existing filters,
tabs, dialogs, CRUD controls, and table columns remain unchanged.

### Empty and error states

Empty states explain the resource status and expose the next relevant command. They do
not use decorative illustrations. Connection failures use semantic warning surfaces and
retain any recovery actions already available.

## Theme Behavior

- Store the selected palette as `cylism-palette` in `localStorage`.
- On first use, select Mint Glass unless the user has a dark system preference, in which
  case select Night Glass.
- Apply the palette attribute before the first application render to avoid a flash of
  the incorrect palette.
- Switching palettes is immediate and does not refetch data or change routes.
- The selector is available in both desktop and mobile navigation.

## Implementation Boundaries

- Keep Vue 3 Composition API and all existing route/API behavior.
- Replace the current top-only navigation with the side rail; preserve every existing
  route and label.
- Move global UI tokens and primitives out of `App.vue` into dedicated stylesheet
  modules so page code uses semantic classes rather than inline color styles.
- Add `lucide-vue-next` for icons if it is not already installed.
- Remove obsolete Soft Tech token names and hard-coded color literals from views.

## Acceptance Criteria

1. All eight pages render correctly in every palette.
2. Palette selection persists across refreshes and is immediately applied.
3. Every glass surface has an opaque fallback when `backdrop-filter` is unavailable.
4. Existing navigation, filtering, CRUD dialogs, API calls, authentication behavior,
   and table content remain functional.
5. Keyboard focus, state contrast, and text readability remain clear in all palettes.
6. Desktop and 375px-wide mobile layouts do not overflow or hide primary actions.

## Visual Reference

Open the local static prototype and choose `06 Pastel Glass`:

`/private/tmp/cylism-ui-directions/index.html`

The prototype is a visual direction only. Production pages will use the structure and
behavior described in this document, not its demonstration markup.
