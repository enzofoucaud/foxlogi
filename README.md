# Foxlogi

A Discord bot for **Foxhole** logistics. Players manually track what they are
crafting; the bot lists active crafts with remaining time and posts a
notification in-channel when a craft is ready — then removes it automatically.

## Requirements

- Go 1.26+ (required by the `modernc.org/sqlite` driver)
- A Discord bot application (see **Discord setup** below)

## Discord setup

### 1. Create the application and bot

1. Go to the [Discord Developer Portal](https://discord.com/developers/applications) → **New Application**.
2. Open the **Bot** tab → **Reset Token** → copy the token into your `.env` as `DISCORD_TOKEN` (keep it secret).
3. Under **Privileged Gateway Intents**, leave everything **OFF** — this bot only uses the
   non-privileged *Guilds* intent. In particular **Message Content is not needed**.

### 2. Required permissions

The bot needs just enough to post craft notifications and embeds:

| Permission    | Why                                          |
| ------------- | -------------------------------------------- |
| View Channels | See the channels where crafts are registered |
| Send Messages | Post the "ready" notification                |
| Embed Links   | Render the `/craft list` embed               |

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

| Variable         | Required | Default      | Description                                             |
| ---------------- | -------- | ------------ | ------------------------------------------------------- |
| `DISCORD_TOKEN`  | yes      | —            | Discord bot token                                       |
| `CRAFT_DB_PATH`  | no       | `foxlogi.db` | Path to the SQLite database file                        |
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

## Deploy with Docker

Everything Docker-related lives in [`docker/`](docker/). The image is a small
(~30 MB) Alpine image wrapping a static, CGO-free binary; the SQLite database is
persisted on the `foxlogi-data` volume.

On the server, create the `.env` (see **Run** above), then:

```sh
make docker-up      # build + start in the background
make docker-logs    # follow logs
make docker-down    # stop and remove
make docker-restart # rebuild + restart after an update
```

Or with Compose directly (run from the project root):

```sh
docker compose -f docker/docker-compose.yml up -d --build
```

The Compose file reads the project-root `.env`, sets `CRAFT_DB_PATH=/data/foxlogi.db`,
and registers commands per guild on startup.

## Makefile

Run `make help` to list all targets:

| Target          | Description                             |
| --------------- | --------------------------------------- |
| `make run`      | Run the bot locally (reads `.env`)      |
| `make build`    | Build the binary                        |
| `make test`     | Run all tests                           |
| `make vet`      | Run `go vet`                            |
| `make fmt`      | Format the code                         |
| `make tidy`     | Tidy `go.mod` / `go.sum`                |
| `make clean`    | Remove build artifacts and local `*.db` |
| `make docker-*` | Build / up / down / restart / logs      |

## Commands

### Craft tracking — `/craft`

- `/craft add item:<name> duration:<30m|1h30m> [quantity:<n>]` — register a craft.
- `/craft list` — list active crafts with remaining time.

When a craft reaches its completion time the bot pings the player in the channel
where it was registered, then removes the craft. There is no manual "done"
command by design.

### Logistics requests — `/request`

A request groups one or more item lines to deliver to a location; anyone can
contribute partial quantities until it's fulfilled.

- `/request new location:<place> [priority:high|medium|low] [deadline:YYYY-MM-DD]` —
  open a request (priority defaults to `medium`).
- `/request additem id:<n> item:<name> quantity:<n>` — add an item line to your
  request (requester only; a repeated item name merges into the existing line).
- `/request list` — list open requests, ordered by priority, with each line's
  `delivered/total` and the delivery location.
- `/request fill id:<n> item:<name> quantity:<n>` — record a contribution to an
  item line (surplus accepted). The requester is pinged on every contribution.
- `/request cancel id:<n>` — cancel your own request (requester only).

When every line of a request is fully delivered, the bot posts a fulfilled
notice pinging the requester and removes the request automatically.

## Development

```sh
go build ./...   # compile everything
go vet ./...     # static checks
go test ./...    # run the storage tests
```

### Architecture

The code is layered so storage and Discord concerns stay decoupled:

- `internal/craft` — domain model `Craft` and its storage-agnostic `Repository`
  interface (craft tracking). Nothing here imports Discord or SQLite.
- `internal/request` — domain models `Request` / `RequestItem` and their
  `Repository` interface (logistics request board). Storage- and Discord-agnostic.
- `internal/storage/sqlite` — implementations of both `Repository` interfaces over
  SQLite (`modernc.org/sqlite`, pure Go / no CGO), sharing one connection.
  `sqlite.go` owns only the shared connection (open, migrate, close); each
  feature's queries live in its own file (`craft.go`, `request.go`). Swappable
  for another backend.
- `internal/bot` — slash commands (the `Command` registry) and the craft
  completion scheduler. Depends only on the `craft.Repository` /
  `request.Repository` interfaces, never on the concrete storage type.
- `internal/config` — environment configuration (Viper).
- `main.go` — the only place that constructs the concrete repository and injects
  it into the commands and scheduler.

### Adding a command

1. Create a type in `internal/bot` implementing the `Command` interface
   (`Definition()` + `Handle()`) — see `craftcmd.go` for a template.
2. Pass an instance to `bot.NewRegistry(...)` in `main.go`.

That's it — registration and interaction dispatch are handled by the registry.

### Database migrations

Schema changes are versioned [goose](https://github.com/pressly/goose) migrations
under `internal/storage/sqlite/migrations/`, embedded via `go:embed` and applied
automatically on `Open` (tracked in the `goose_db_version` table).

To add one, create the next numbered file, e.g.
`internal/storage/sqlite/migrations/0002_add_xxx.sql`:

```sql
-- +goose Up
ALTER TABLE crafts ADD COLUMN note TEXT;

-- +goose Down
ALTER TABLE crafts DROP COLUMN note;
```

It runs on the next startup. Use strict DDL (the `0001` baseline only uses
`IF NOT EXISTS` to stay safe on pre-migration databases).
