[العربية](../../guides/organization.md)

# Your organization and your account

The **Organization** group in the side menu holds Members, Teams, Invitations, API keys and Organization settings. Your own account is at the bottom of the side menu: Profile, Security and Preferences. Each page has at most one primary button, and each list shows a skeleton while it loads, an error with **Try again** when it cannot load, and a plain sentence when it is empty.

## Who can do what

| Page or action | Owner | Admin | Maintainer | Developer | Viewer |
|---|---|---|---|---|---|
| See members and teams | ✓ | ✓ | ✓ | ✓ | |
| See a member's second step | ✓ | ✓ | | | |
| Change a role, reset a second step, remove a member | ✓ | ✓ (not owners) | | | |
| Create, edit and delete teams, add and remove team members | ✓ | ✓ | | | |
| Invitations, organization API keys | ✓ | ✓ | | | |
| Rename the organization | ✓ | ✓ | | | |
| Change the second-step rule, transfer ownership | ✓ | | | | |
| Leave the organization | ✓ | ✓ | ✓ | ✓ | ✓ |

A page your role cannot open shows the not-found page. A button you cannot use is not shown.

## Members (`/org/members`)

The list shows each member's name, email, role and join date. Owners and admins also see a badge for a missing second step: amber when the organization requires one for that role, grey otherwise.

Pick a member to open their details (`/org/members/<id>`):

- **On screens 1280px wide and up,** the details open beside the list. The selected row is highlighted, and the list keeps its place and page while you move between members.
- **On smaller screens,** the details take the whole page, with **All members** at the top to go back.

The details show the role (and the higher role a team grants, if any), the second step (owners and admins only), the member's teams, each a link to the team, and the join date. The teams come with the members list (`Teams` on each member), so opening a member sends no extra request. Owners and admins can also:

- **Change the role** from a list. Giving or taking the owner role asks for your current factor. The member is signed out everywhere so the new role applies at once.
- **Reset the second step** of a member who lost every factor and recovery code.
- **Remove the member.** A slide asks you to confirm before anything is sent. An owner must be given another role first.

On your own row, the page points to your Security settings instead.

Owners and admins also see when the member was last active in this organization: the last signed-in request to one of its pages, recorded at most once a minute. Activity in another organization does not count, and a member who has not used this organization yet shows **No recorded activity in this organization**. It comes with the members list (`LastActiveAt` on each member, only for callers with `member.manage`).

## Teams (`/org/teams`)

A team groups members and can grant them one role in the organization; a member gets the higher of their own role and their teams' roles. Teams work like members: the team opens beside the list from 1280px up, and alone with **All teams** below that. Owners and admins can rename a team, add and remove members, change its role and delete it. **New team** is the page's primary button.

## Invitations (`/org/invitations`)

One list holds the pending invitations first and the expired ones after them, each with its role, its expiry date and who sent it. **New link** replaces the link with a fresh one valid for 7 days, so the old one stops working, and emails it when email is set up (at most 5 sends per invitation). **Revoke** stops the link at once. **Invite a member** opens the invite slide. Accepted invitations are not listed: the member appears in Members instead. The API can list them with `GET /api/v1/organizations/<id>/invitations?Status=accepted`, newest first, with when each was accepted (`AcceptedAt`) and who accepted it (`AcceptedByUserId`, `AcceptedByName`). It needs the same permission as the list.

## API keys (`/org/api-keys`)

Every API key a member created in the organization, with its scopes, creator, creation date, last use and expiry. Owners and admins can revoke any of them; **New key** opens the create slide.

## Organization settings (`/org/settings`)

- **General:** the name (owners and admins can change it), the short link (slug) and the creation date.
- **Security:** who must use a second step. Only an owner can change the rule; everyone else sees it.
- **Job log retention:** 7 to 3650 days, or empty to follow the server setting. Only an owner changes it, with their second step; everyone else sees it. Older logs stop showing at once, and the server setting wins when it is shorter.
- **Danger zone:** transfer ownership (owners only) and leave the organization. Each opens a slide that asks you to confirm. If you are the last owner, the server refuses to let you leave until you transfer the ownership.

## Your account

- **Profile (`/account/profile`):** your name, email, avatar initial, language and your role in the current organization. The name and email cannot be changed from the web app yet.
- **Security (`/account/security`):** a status line at the top is amber until you turn on a second step, and **Turn on the second step** is the page's primary button until then. The page has four sections:
  - **Password and sign-in:** change the password (your other devices are signed out) and link or unlink single sign-on accounts.
  - **Second step:** authenticator codes, recovery codes and passkeys.
  - **Signed-in devices:** sign out one device, or every device but this one.
  - **Personal API tokens:** keys that act as you in one organization, never with more than your role.
- **Preferences (`/account/preferences`):** the language and the theme. The theme editor opens in a slide.

## Reaching any setting

All the settings pages follow one order: **General** first, then the sections of the page itself, then **Security**, then **Notifications**, and **Danger zone** last. The section names are the same on every page. At the top of each page, a list called **On this page** takes you to the section you want.

- Press Ctrl K or ⌘K and type the name of the setting, such as "Security" or "Language". Every setting your role allows is in the palette, and it opens the page at its section.
- The settings and notifications of a project show in the palette while a project is open.
- What your role does not allow does not show in the palette or in the list.
- The notification bell updates by itself when a new notification arrives or when you read one in another tab. If the live stream drops, the bell asks once a minute.

## Names in mixed text

Names, emails, team names and addresses are isolated from the text around them, so an Arabic name in the English interface (or an English one in Arabic) keeps the sentence in order, for example `صبري داود · 127.0.0.1 · 1 minute ago` in the audit log.
