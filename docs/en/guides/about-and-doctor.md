[العربية](../../guides/about-and-doctor.md)

# About Dhow and the server checks

The **About Dhow** page (`/about`, under **Monitoring** in the side menu) shows the version that runs now, the links to the docs, and the license. The instance owner also sees the server checks that `dhow-server doctor` prints, with the same fix sentences. The decision is [ADR-0112](../../../Plan/ADR/0112-about-page-and-server-doctor.md).

## Who sees what

Every signed-in user sees **Version and build**: the version number, the commit, the build date, and whether the server is ready. The **Server checks** card is for the instance owner only, the account created in the first setup. The server sends no check to an ordinary member, neither in the page nor in the API. An API key cannot read this page with the instance owner's rights.

## Server checks

Each check has a state shown with a word and an icon, not with color alone: **Healthy**, **Note**, **Warning**, or **Failed**. When the state is not healthy, a **Fix:** line appears under the check. It is the same sentence `dhow-server doctor` prints, in the language of the page.

| Check | What it checks |
|---|---|
| Database | That the server reaches PostgreSQL |
| Database migrations | That the shape of the database fits this version |
| Server settings reload | That the last reload of the settings worked |
| Keyring | That the key file holds every key version the database needs |
| Today's log storage | That today's log table exists |
| Free disk space for the server data | Warning under 5 GiB and failure under 1 GiB |
| Server clock against the database clock | Warning at a 5 second difference and failure at 30 seconds |
| Kernel MPTCP | Warning while MPTCP is on (CVE-2026-98432) |

Checks that need root on the host, such as file permissions and the update timer, do not run inside the server. Run them with `dhow-server doctor` on the host. See the [installer](../install/installer.md).

## What never appears

The server never returns a secret value, a setting value, a file path, or a database error text. It returns the name of the check, its state, and the fix sentence only. A test in the code plants secret values and paths and checks that none of them is in the response.

## The reload bar

If the server was updated after you opened the page, a bar appears at the top: **A newer version of Dhow is running** with a **Reload the page** button. The web app compares the version, the commit and the build date it got when it opened with what `GET /api/v1/meta` returns now. It asks once every five minutes only, and stops while the tab is hidden. If the server is down during the update, it retries every ten seconds until the server is back.

## License

Dhow is free software under AGPL-3.0. The commercial features under the `ee/` folder have a separate license, the Dhow Enterprise License. The details are in [Editions and licensing](../editions-and-licensing.md).

## The API

`GET /api/v1/system/doctor` answers with `ServerBuild` (`Version`, `Commit` and `BuiltAt`) and `Health`, and with a `Checks` field for the instance owner only. The language comes from the `Accept-Language` header. `GET /api/v1/meta` also answers with `ServerBuild`. The build date is the time of the commit the binary was built from, so the build stays reproducible with the same output.
