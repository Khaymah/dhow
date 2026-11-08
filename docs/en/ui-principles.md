[العربية](../ui-principles.md)

# UI principles

Status: **Accepted**. The current visual direction is "control room" (ADR-0059, which amends ADR-0034).

Decisions: ADR-0021 to ADR-0023, ADR-0034, ADR-0059 and ADR-0062.

## 1. Arabic-first, not "mirrored English" (ADR-0021)

- The **primary product language is Arabic**, written in **simple Arabic** (ADR-0062): Modern Standard grammar, short sentences and words any Arab reader understands, neither Egyptian colloquial nor heavy classical. This covers the UI, server messages and emails, and the installer and `doctor` output.
  - Buttons say exactly what they do (أضف جهاز تشغيل، احفظ التغييرات). Errors say what happened, then what to do (لم نتمكن من حفظ التغييرات. حاول مرة أخرى.).
  - A project hook rejects colloquial markers (such as دلوقتي، مش، اللي، مفيش) and heavy classical ones (such as يتعذّر، ريثما) in the Arabic message catalogs.
  - English is the second first-class language.
  - Further languages come later through community translation.
- **Screens are designed RTL first**, then adapted to LTR. Layout, iconography direction, reading order, number and date formatting, and empty states are all considered natively in Arabic.
- Engineering rules:
  - Use **CSS logical properties only** (`margin-inline-start`, `inset-inline-end`, …). Lint rules forbid physical left/right.
  - Icons that imply direction (arrows, chevrons, "back") are mirrored via `dir`. Icons that don't imply direction are never mirrored.
  - **Never hardcode strings.** CI fails on missing keys in any shipped locale.
  - Mixed-direction content gets proper bidi isolation: commit SHAs, branch names, paths and logs render LTR inside RTL layouts using `<bdi>` / `dir="ltr"`.
  - Logs and code are always LTR, in monospace.
- **Typography:** IBM Plex Sans Arabic for Arabic, IBM Plex Sans for Latin and numerals, and IBM Plex Mono for code and logs. All three are self-hosted (SIL OFL 1.1) in weights 400, 500 and 600.
  - Arabic has its own sizes, set on `:root:lang(ar)` by the generated tokens, so no class is needed.
  - The scale, Latin / Arabic in px (size / line height):

    | Role | Latin | Arabic |
    |---|---|---|
    | Page title | 24 / 34 | 26 / 38 |
    | Section and slide title | 18 / 28 | 19 / 30 |
    | Card title | 16 / 24 | 17 / 26 |
    | Body | 15 / 24 | 16 / 28 |
    | Secondary text | 14 / 22 | 15 / 24 |
    | Buttons and labels | 14 / 20 | 15 / 22 |
    | Small buttons | 13 / 20 | 14 / 20 |
    | Badges and dates | 13 / 20 | 13 / 20 |
    | Stats | 30 / 38 | 30 / 38 |
    | Code and logs | 13 / 22 | 13 / 22 |

  - Nothing is smaller than 13px, and 13px is only for badges, dates and code. Titles go up to weight 600.
- **Spacing:** a 4px grid. The page gutter is 32px (16px on phones), cards have 20px of padding (14px on phones), blocks are 16px apart, and the top bar is 60px high.

## 2. Navigation model (ADR-0022, ADR-0059)

- **Every section is a full page with its own URL.** The shell has three parts: a side menu, a top bar and the page.
- **Side menu** (248px wide), grouped in this order. Group titles are labels, not links, and the current item is marked with `aria-current="page"`. Items a role cannot open are hidden.
  1. Home (`/`).
  2. **Run:** Jobs (`/jobs`) and Runners (`/runners`).
  3. **Organization:** Members (`/org/members`), Teams (`/org/teams`), Invitations (`/org/invitations`), API keys (`/org/api-keys`) and Organization settings (`/org/settings`).
  4. **Monitoring:** Audit log (`/audit`) and Server status (`/system`).
  5. The account at the bottom: avatar, name and role, and sign out. It opens Profile (`/account/profile`), and the account pages link to Security (`/account/security`) and Preferences (`/account/preferences`, appearance, themes and language).
  - Projects join the menu, right after Home, with M4. Until then they are hidden.
  - **Counters** sit next to the items that need attention: Jobs shows the queued jobs (in the warning tone when some have no matching runner), Runners the offline runners and Invitations the pending ones. A counter is hidden at zero, while loading and when the role may not read it, and screen readers hear what it counts. On phones, a dot on Jobs means jobs wait with no matching runner. See [The home page](guides/home.md).
- **Detail pages:** a job (`/jobs/<id>`), a runner (`/runners/<id>`), a member (`/org/members/<id>`), a team (`/org/teams/<id>`), an audit event (`/audit/<id>`) and a server check (`/system/checks/<name>`). Runners also has Registration tokens (`/runners/tokens`). The organization switcher is at `/organizations`.
  - Ids in the address are checked. An invalid or unknown id shows the not-found state.
  - A page the role cannot open shows the not-found state, never an empty page, and asks the server for nothing.
- **Every page has a header:** the title, one line of context, and at most one primary action.
- **Top bar** (60px): the page path (for example "Run › Jobs › Job details"), search, which opens the command palette (Ctrl K / ⌘K), a server status dot that links to `/system`, notifications and the language switch.
- **Phones** (below the `md` breakpoint): the side menu is hidden and a bottom bar shows five items, each with an icon and its name: Home, Jobs, Runners, Organization (opens Members, or the first organization page the role can open) and Account. Pages of the organization and of the account link to their sibling pages at the top.
- **Slides are for create and confirm only:** invite a member, new team, new organization API key, new registration token, new personal API key, change the password, set up TOTP, transfer ownership, remove a member, leave the organization, new organization and edit a theme. They are 560px wide and open over the current page. The open slide is part of the URL (`?slide=<id>`), so a reload or a shared link opens it again, and the browser back button closes it.
- **Old links keep working.** A link from before this change (`/?stack=…`) is replaced in the history with the matching page, and a create slide in it opens over that page.
- **Command palette** (Ctrl K / ⌘K): every page the role can open, every setting the role allows (it opens the page at its section), the create actions (each opens its slide over its page), sign out and the language switch.
- **Motion:** transitions are short slides of 150–250 ms with eased curves, and they respect `prefers-reduced-motion`.

## 3. Calm, low-density visual style

- The style is minimal: generous whitespace, strong typographic hierarchy, one accent color, and status shown with calm semantic colors **plus** icons and text (never color alone).
- **Progressive disclosure:**
  - Summaries come first and details appear on demand.
  - Tables are not the default view.
  - Show the ~5 things that matter and put the rest on a detail page.
- Four dark system themes, plus custom ones. Text meets WCAG AA on every surface of every theme, and cards stand out with a hairline instead of a shadow.
- Status has its own colors, each with a background and a foreground: green for success, blue for running, violet for queued, amber for warnings and coral for failures.

## 4. Performance budgets

| Budget | Target |
|---|---|
| Initial JS (gzip) on first route | < 150 KB |
| Route-level code splitting | Mandatory, including charts and editors, which load lazily |
| Interaction response (INP) | < 100 ms |
| LCP on a typical VPS-hosted install | < 1.5 s |
| Log viewer | Virtualized; smooth at 100k+ lines; follow-tail toggle |
| Live updates | Patch caches incrementally from SSE events; no refetch storms |

## 5. Stack (ADR-0023)

- React with Vite and TypeScript.
- Tailwind CSS (logical utilities), with **shadcn/ui** components built on Radix primitives and owned in `packages/ui`.
- **Motion** for transitions.
- TanStack Query (object API) for server state. The API client is generated from OpenAPI.
- i18next, with ICU messages for Arabic plural forms.
- Accessibility target: **WCAG 2.2 AA**. Requirements include full keyboard support, visible focus, labelled icon buttons, `role="log"` with `aria-live` on live logs, and contrast checked in both themes.

## 6. Process

- **The design system comes before implementation.** Tokens, typography, components and key screens are reviewed and signed off before any production UI is built.
- Every component ships with RTL and LTR stories, both themes, and visual-regression tests.
