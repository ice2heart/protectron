# protectron

Telegram anti-spam bot: new members are muted and get an image captcha
(Cyrillic or Latin, per chat language) with shuffled inline-keyboard buttons.
Pass — unmuted and welcomed; fail or time out — kicked with a short ban.

Written in Go on [go-telegram/bot](https://github.com/go-telegram/bot),
state in MongoDB. Design docs: [doc/architecture.md](doc/architecture.md),
[doc/plan.md](doc/plan.md).

## Build

```sh
go build ./cmd/protectron          # local binary
docker compose build protectron    # scratch-based image
```

## Run

The bot needs admin rights in the group with the "restrict members"
permission, and the `chat_member` update requires long polling with
`allowed_updates` — handled automatically on start.

Create `protectron.env` next to `docker-compose.yml`:

```
API_TOKEN=123456:telegram-bot-token
ADMIN_ID=11111111
```

then:

```sh
docker compose up -d protectron
```

`MONGO_URI` is set in the compose file (shared `mongodb` service). If your
Mongo has auth enabled, put the credentials into the URI.

### Configuration (env)

| Var | Meaning | Default |
|---|---|---|
| `API_TOKEN` | bot token | required |
| `MONGO_URI` | e.g. `mongodb://mongodb:27017` | required |
| `MONGO_DB` | database name | `protectron` |
| `ADMIN_ID` | super admin: greeted instead of captcha'd, may call `/stats` | unset |
| `LOG_LEVEL` | `debug` / `info` / `warn` / `error` | `info` |

## Commands

In-group, chat admins only:

```
/settings                 show per-chat settings
/set lang ru|en
/set timeout <60..3600>   captcha timeout, seconds
/set length <4..10>       captcha length, chars
/set attempts <1..5>      attempts before kick
/set ban <30..86400>      ban duration after fail/timeout, seconds
```

Anyone: `/ping`. Super admin, in a private chat with the bot: `/stats`
(per-chat usage counters, all time + last 7 days).

## Tests

```sh
go test ./...                                        # unit tests
MONGO_TEST_URI=mongodb://localhost:27017 go test ./internal/storage/   # + repo tests
```
