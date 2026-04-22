# Third-Party Licenses

NRFLO includes software from the following open source projects. We are
grateful to the authors and maintainers of these libraries.

Each project listed below is used as a dependency, unmodified, and retains
its original license terms. Full license texts are available at the linked URLs.

## Go Backend Dependencies

| Package | License | Source |
|---------|---------|--------|
| fyne.io/systray | Apache-2.0 | https://github.com/fyne-io/systray |
| github.com/creack/pty | MIT | https://github.com/creack/pty |
| github.com/dustin/go-humanize | MIT | https://github.com/dustin/go-humanize |
| github.com/godbus/dbus/v5 | BSD-2-Clause | https://github.com/godbus/dbus |
| github.com/golang-migrate/migrate/v4 | MIT | https://github.com/golang-migrate/migrate |
| github.com/google/uuid | BSD-3-Clause | https://github.com/google/uuid |
| github.com/gorilla/websocket | BSD-2-Clause | https://github.com/gorilla/websocket |
| github.com/mattn/go-isatty | MIT | https://github.com/mattn/go-isatty |
| github.com/ncruces/go-strftime | MIT | https://github.com/ncruces/go-strftime |
| github.com/remyoudompheng/bigfft | BSD-3-Clause | https://github.com/remyoudompheng/bigfft |
| github.com/spf13/cobra | Apache-2.0 | https://github.com/spf13/cobra |
| github.com/spf13/pflag | BSD-3-Clause | https://github.com/spf13/pflag |
| golang.org/x/image | BSD-3-Clause | https://cs.opensource.google/go/x/image |
| golang.org/x/sys | BSD-3-Clause | https://cs.opensource.google/go/x/sys |
| golang.org/x/text | BSD-3-Clause | https://cs.opensource.google/go/x/text |
| modernc.org/libc | BSD-3-Clause | https://gitlab.com/cznic/libc |
| modernc.org/mathutil | BSD-3-Clause | https://gitlab.com/cznic/mathutil |
| modernc.org/memory | BSD-3-Clause | https://gitlab.com/cznic/memory |
| modernc.org/sqlite | BSD-3-Clause | https://gitlab.com/cznic/sqlite |

## Frontend Dependencies

Frontend dependencies are declared in `ui/package.json`. Major packages include:

| Package | License | Source |
|---------|---------|--------|
| React | MIT | https://github.com/facebook/react |
| TypeScript | Apache-2.0 | https://github.com/microsoft/TypeScript |
| TanStack Query | MIT | https://github.com/TanStack/query |
| Zustand | MIT | https://github.com/pmndrs/zustand |
| Tailwind CSS | MIT | https://github.com/tailwindlabs/tailwindcss |
| xterm.js | MIT | https://github.com/xtermjs/xterm.js |
| React Flow | MIT | https://github.com/xyflow/xyflow |
| CodeMirror 6 | MIT | https://github.com/codemirror |
| Zod | MIT | https://github.com/colinhacks/zod |

For a complete list, see `ui/package.json` and run `npm list --all` or inspect
`node_modules/*/LICENSE` files in the frontend build.

## External Tools (invoked as subprocess)

NRFLO invokes the following CLI tools as external processes when users
configure them. These tools are not bundled or redistributed with NRFLO:

- **Claude CLI** by Anthropic
- **Codex CLI / ChatGPT CLI** by OpenAI
- **Opencode**

Users must install and authenticate these tools separately according to
their respective terms of service.

## Trademark Notice

"Claude" and "Claude Code" are trademarks of Anthropic, PBC. "Codex" and
"ChatGPT" are trademarks of OpenAI, L.P. NRFLO is an independent project
and is not affiliated with, endorsed by, or sponsored by Anthropic or OpenAI.
