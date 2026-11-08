[العربية](../../guides/help-and-faq.md)

# Help inside Dhow

You find help inside Dhow in three places: the Help button in the header of every page, the **Help** page with its frequently asked questions, and the **What's new** card on the home page.

## The Help button on every page

The header of every page has a **Help** button. It opens the page's documentation in a new tab, in the interface language you use. When you hover over the button or focus it, a short line appears that explains what the page does, and screen readers read it.

Every page in Dhow has a documentation file in Arabic and in English. A test in the web app checks this: if a page is added without a documentation file in both languages, the test fails.

## The Help page

Open the **Help** page from the Help button in the top bar or from the command palette. It has:

- **Search** over the frequently asked questions and the troubleshooting sections. Type a word or a short phrase, and only the matching questions stay.
- **Frequently asked questions**, grouped by topic.
- **Troubleshooting**: chosen sections of [Troubleshooting](../install/troubleshooting.md).
- **What's new in this version.**

The content is embedded in the web app, so search and reading work without an internet connection, even on an isolated network. Links to the full documentation need a connection.

## The What's new card

After Dhow is upgraded, the home page shows a **What's new** card once per user. It lists the main changes of the version. The server owner also sees the **Actions required** section when there is one. When you close the card, Dhow saves on your account that you read this version's notes, so it does not show again on any device.

## Where the content comes from

These pages have no hand-written text inside the web app. The `pnpm gen:help` command in `apps/web` turns the documentation files into TypeScript files generated at build time:

- The [frequently asked questions](../faq.md) in `docs/faq.md`.
- Chosen sections of [Troubleshooting](../install/troubleshooting.md).
- Release notes in `docs/releases/X.Y.Z.md`, with fixed headings: "Highlights", "Security fixes", "Actions required", and "Deprecations".

The generator accepts only headings, paragraphs, lists, links and code. It rejects raw HTML, images, tables, and links that are not allowed, such as `javascript:` and `data:`, and it accepts links only to `.md` files in `docs/` and to Dhow's repository on GitHub. It runs on both languages, and it fails if they differ in the number of questions or sections. CI checks that the generated files are current, and nobody edits them by hand.

## Adding a question or a release

- To add a question, add it to `docs/faq.md` and `docs/en/faq.md` under a `###` heading inside a `##` group, then run `pnpm gen:help`.
- To add a release, create `docs/releases/X.Y.Z.md` and `docs/en/releases/X.Y.Z.md`, then run `pnpm gen:help`. The newest release shows in the What's new card.
