[العربية](../../guides/accessibility.md)

# Accessibility

We want everyone to be able to use Dhow: with the keyboard only, with a screen reader, with the page zoomed in, or with reduced motion. This page explains what the interface supports today, how we check it, and how to report a problem.

## What the interface supports

| Feature | What it does |
|---|---|
| Keyboard | Every button, link and field can be reached with Tab and activated with Enter or Space. Tabs move with the arrow keys, Home and End. |
| Visible focus | A blue outline shows around the focused element in every theme, and no fixed bar or panel hides it. |
| Skip to content | The first element Tab reaches is a "Skip to content" link. |
| Button names | Every icon button has a name that a screen reader reads out. |
| Target size | Every clickable element is at least 24 pixels, and buttons are 44 pixels on a phone. |
| Contrast | Normal text has a contrast of at least 4.5 to 1 in all four system themes, and an automated test checks it. |
| Language and direction | Arabic right to left is the default, English is left to right, and the `lang` and `dir` attributes follow the language. |
| Motion | When "reduce motion" is on in the system, animations and transitions stop. |
| Help place | The help button has a fixed place in every page header. It is empty until in-app help arrives. |

## Keyboard shortcuts

| Shortcut | Action |
|---|---|
| `Ctrl+K` or `Cmd+K` | Open the command palette and search |
| `Tab` and `Shift+Tab` | Move to the next or previous element |
| `Enter` or `Space` | Activate the button or the checkbox |
| `Esc` | Close the panel or dialog |
| Arrows, `Home` and `End` | Move between tabs |

## How we check

- **Automated check on every change:** the test `E2E/Accessibility.spec.ts` runs the axe-core library in the browser on the sign-in, home, projects, run, environment, members and settings pages. It checks them in Arabic and English and in the four system themes, and fails on any serious or critical violation. Moderate and minor violations are recorded in the test report and do not fail it.
- **Unit tests:** they check button names, checkbox size, that the focus outline stays, and the Tab order in the core components.
- **Manual review before the first release:** an automated check does not catch everything, such as focus order and what a screen reader says. So a person runs a manual checklist and signs it with their name, project and date, and the first release does not ship before that. The checklist is in `Plan/A11y-Manual-Checklist.md`.

## Known limits

- The interface has not yet been tried with screen readers. It will be in the manual review, and the result will be written here.
- A group of choice buttons stops at every option with Tab instead of moving with the arrows inside the group. Using it is possible but takes longer.
- All four system themes are dark. There is no light theme yet.

## Report a problem

If you find something you cannot use, open an issue on the [repository](https://github.com/Khaymah/dhow/issues) with a title that starts with "Accessibility". Write the page, the language, the theme, the browser, the assistive tool if any, what you expected and what happened. Do not write secrets or sensitive data.

Security problems are never written in public issues. See [SECURITY.en.md](../../../SECURITY.en.md).
