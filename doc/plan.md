# Protectron — Go Rewrite Plan

Execution plan for the port described in [architecture.md](architecture.md). Each phase leaves the repo in a working, testable state.

> **Status (2026-07-15):** Phases 0–6b implemented (`go build`/`go vet`/`go test` green; storage tests need `MONGO_TEST_URI`). Outstanding: the Phase 4 manual e2e milestone in a scratch group, restart-persistence verification (Phase 5), and the Phase 7 cutover steps (run alongside the real group, then delete the Python sources).

## Phase 0 — Spike: captcha image with Cyrillic

The one real technical risk. Before committing to the structure:

- [x] Tiny throwaway `main.go`: render `бвгдж23` with `steambap/captcha` + a Cyrillic-capable TTF (try Noto Sans TTF, DejaVu Sans), save PNG, eyeball it.
- [x] If freetype rejects the font or the output is ugly ⇒ fall back to a custom renderer (`fogleman/gg`: draw glyphs with jitter/rotation, add noise curves and dots manually).
- [x] Decision recorded here; chosen font committed to `assets/`.

**Decision (2026-07-15):** custom renderer with `fogleman/gg` + `DejaVuSans-Bold.ttf`. `steambap/captcha` is disqualified outright: its `randomText` indexes `CharPreset` by **byte**, so multi-byte UTF-8 (Cyrillic) text generation is broken, and `New` accepts no caller-supplied text. DejaVu Sans Bold parses cleanly with `golang/freetype` and Cyrillic renders legibly with per-glyph rotation/jitter/color plus noise curves and dots. Font (+ its license) committed to `assets/`.

## Phase 1 — Skeleton

- [ ] `go mod init github.com/ice2heart/protectron`, Go 1.24+.
- [ ] `internal/config`: env parsing; `cmd/protectron`: slog setup, Mongo connect with ping, `bot.New` with `WithAllowedUpdates` including `chat_member`, `message`, `callback_query`; graceful shutdown on SIGINT/SIGTERM.
- [ ] `/ping` handler answering `pong_msg` — proves polling + i18n end to end.
- [ ] `internal/i18n`: load `templates/*.json`, `${var}` substitution, tests.

## Phase 2 — Storage

- [ ] `ChatRepo`: get-with-defaults, upsert settings, index creation on startup.
- [ ] `SessionRepo`: insert, get by id, atomic input push / backspace pop (`findOneAndUpdate`), set join message id, delete, list expired.
- [ ] Unique index `(chat_id, user_id)`, index on `expires_at`.
- [ ] Tests against a real Mongo (testcontainers or a compose-provided instance).

## Phase 3 — Captcha core

- [ ] `internal/captcha`: charset per language, `crypto/rand` text sampling, image rendering (per Phase 0), token generation, keyboard building (2×4 + backspace).
- [ ] Unit tests: token uniqueness, duplicate answer chars handled, keyboard shape, callback-data length < 64 bytes.

## Phase 4 — Join & callback flows

- [ ] `chat_member` handler: joined → skip self/bots/ADMIN_ID → mute → send captcha → create session; NotEnoughRights → warn + leave chat.
- [ ] `new_chat_members` handler: record join message id only.
- [ ] Callback handler: full validation chain, per-session mutex, success / retry / final-fail branches per architecture doc.
- [ ] Leave / manual-unrestrict handler: cancel session, clean messages.
- [ ] Message cleanup helper tolerant of already-deleted messages.

**Milestone:** end-to-end manual test in a scratch group — join with a second account, pass, fail, retry, leave mid-captcha.

## Phase 5 — Sweeper & retries polish

- [ ] `internal/sweeper`: 30 s ticker, expired sessions → kick + ban + clean + delete; per-item error isolation.
- [ ] Verify restart persistence: kill bot mid-captcha, restart, buttons still work (session state is fully in Mongo).

## Phase 6 — Admin commands

- [ ] `/settings`, `/set …` with validation ranges; admin check via `getChatMember` + small TTL cache.
- [ ] New template keys in `ru.json` / `en.json` (`retry_msg`, `settings_msg`, `set_ok_msg`, `set_bad_value_msg`, `admins_only_warn`).

## Phase 6b — Usage statistics

- [ ] `StatsRepo`: `$inc` upsert per (chat, UTC day) for joins / passed / failed / timeouts / leaves; fire-and-forget from the flows (log, never fail the captcha path).
- [ ] Wire counters into join, callback success/final-fail, sweeper, and leave paths.
- [ ] `/stats` in private chat, super admin (`ADMIN_ID`) only: per-chat totals, all time + last 7 days.
- [ ] Tests: repo increments, aggregation query.

## Phase 7 — Packaging & cutover

- [x] Multi-stage Dockerfile: static build on `golang:alpine`, final stage **`scratch`** (user decision — the server already runs another bot from this compose file). Font is `go:embed`-ded; only `templates/` and CA certs are copied in.
- [x] `docker-compose.yml`: protectron added **next to the existing `poke_bot` service**, sharing the existing `mongodb` (mongo:8.2) service and volume; own `protectron.env` to avoid variable collisions with poke_bot's `.env`.
- [x] README rewrite: build, run, configuration table.
- [ ] Run the Go bot in the real group alongside monitoring for a few days.
- [ ] Delete Python sources (`protectron.py`, `src/`, `configs.py`, `hash_check.py`, `Pipfile*`, `.otf` if replaced), keep `templates/`.

## Out of scope (deliberately)

- Webhook transport, horizontal scaling (per-session mutex assumes one instance).
- Migration of `db.json` (ephemeral data only).
- Video/math captcha (`video_captcha` in the old code was experimental and unused by the main flow).
- Decoy buttons, HMAC-signed payloads, wrong-click penalties — possible later hardening, not in v1.
