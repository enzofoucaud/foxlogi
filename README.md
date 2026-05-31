# Foxlogi

A Discord bot for **Foxhole** logistics. Players manually track what they are
crafting; the bot lists active crafts with remaining time and posts a
notification in-channel when a craft is ready — then removes it automatically.

## Requirements

- Go 1.25+ (required by the `modernc.org/sqlite` driver)
- A Discord bot application (see **Discord setup** below)

## Discord setup

### 1. Create the application and bot

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications) → **New Application**.
2. Open the **Bot** tab → **Reset Token** → copy the token into your `.env` as `DISCORD_TOKEN` (keep it secret).
3. Under **Privileged Gateway Intents**, leave everything **OFF** — this bot only uses the
   non-privileged *Guilds* intent. In particular **Message Content is not needed**.

### 2. Required permissions

The bot needs just enough to post craft notifications and embeds:

| Permission       | Why                                                     |
|------------------|---------------------------------------------------------|
| View Channels    | See the channels where crafts are registered            |
| Send Messages    | Post the "ready" notification                            |
| Embed Links      | Render the `/craft list` embed                           |

These three add up to the permissions integer **`19456`**. Slash commands
themselves are granted by the `applications.commands` OAuth2 scope, not a
permission bit.

### 3. Invite the bot

In the Developer Portal, open **OAuth2 → URL Generator**:

- **Scopes:** `bot` and `applications.commands`
- **Bot Permissions:** *View Channels*, *Send Messages*, *Embed Links*

Copy the generated URL, open it, choose your server, and authorize. Or build it
manually (replace `YOUR_CLIENT_ID` with the application's Client ID from the
**OAuth2** tab):

```
https://discord.com/api/oauth2/authorize?client_id=YOUR_CLIENT_ID&scope=bot%20applications.commands&permissions=19456
```

> Inviting requires the **Manage Server** permission on the target guild. After
> inviting, make sure the bot's role can view and send messages in the channels
> where players will run `/craft`.

Slash commands are registered per guild on startup, so they appear within
seconds of the bot joining (no need to wait for global propagation).

## Configuration

| Variable         | Required | Default      | Description                                            |
|------------------|----------|--------------|--------------------------------------------------------|
| `DISCORD_TOKEN`  | yes      | —            | Discord bot token                                      |
| `CRAFT_DB_PATH`  | no       | `foxlogi.db` | Path to the SQLite database file                       |
| `SOON_THRESHOLD` | no       | `10m`        | Crafts within this window are flagged `⏳ soon` in lists |

## Run

Create a `.env` file in the project root (it is git-ignored):

```env
DISCORD_TOKEN=your-token-here
# optional:
# CRAFT_DB_PATH=foxlogi.db
# SOON_THRESHOLD=10m
```

Then:

```sh
go run .
```

Configuration is loaded via [Viper](https://github.com/spf13/viper): real
environment variables take precedence over the `.env` file, which takes
precedence over built-in defaults. So `export DISCORD_TOKEN=...` also works.

Build a binary:

```sh
go build -o foxlogi .
./foxlogi
```

## Commands

- `/craft add item:<name> duration:<30m|1h30m> [quantity:<n>]` — register a craft.
- `/craft list` — list active crafts with remaining time.

When a craft reaches its completion time the bot pings the player in the channel
where it was registered, then removes the craft. There is no manual "done"
command by design.

## Development

```sh
go build ./...   # compile everything
go vet ./...     # static checks
go test ./...    # run the storage tests
```

### Architecture

The code is layered so storage and Discord concerns stay decoupled:

- `internal/craft` — domain model `Craft` and the storage-agnostic `Repository`
  interface. Nothing here imports Discord or SQLite.
- `internal/storage/sqlite` — `Repository` implementation over SQLite
  (`modernc.org/sqlite`, pure Go / no CGO). Swappable for another backend.
- `internal/bot` — slash commands and the completion scheduler. Depends only on
  `craft.Repository`, never on the concrete storage type.
- `internal/config` — environment configuration.
- `main.go` — the only place that constructs the concrete repository and injects
  it into the bot.

### Adding a command

1. Create a type in `internal/bot` implementing the `Command` interface
   (`Definition()` + `Handle()`) — see `craftcmd.go` for a template.
2. Pass an instance to `bot.NewRegistry(...)` in `main.go`.

That's it — registration and interaction dispatch are handled by the registry.
