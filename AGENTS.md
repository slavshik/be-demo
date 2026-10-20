# AGENTS.md

Mini Game Backend API: player registration, scores, and leaderboard.
Prioritize **correctness > simplicity > readability**; keep Go idiomatic and minimal.

## Stack & Layout

Go 1.22+, standard `net/http`, `database/sql`, and `modernc.org/sqlite` (only external dependency).
No auth, frameworks, ORMs, Redis, queues, WebSockets, repository interfaces, or packages beyond those listed below.

- `cmd/server/main.go`: configuration, server, graceful shutdown.
- `internal/store/store.go`: schema, queries, pragmas, sentinel errors.
- `internal/api/handlers.go`: method-based ServeMux routes, validation, JSON helpers.
- `internal/api/api_test.go`: HTTP tests using `testing`, `httptest`, and SQLite files in `t.TempDir()`.
- `internal/web/`: `index.html` (inline CSS and plain JS, no build step or npm) embedded with `embed` and served at `GET /{$}` (ADR 7). It uses only the public JSON API; render user data with `textContent`, never `innerHTML`.

## API

All bodies are JSON. Errors: `{"error":"short, lowercase, specific message"}`.
Reject malformed JSON, unknown fields, and trailing content with 400; limit bodies to 1 MB using `http.MaxBytesReader`, `DisallowUnknownFields()`, and an EOF check.
Use `writeJSON`/`writeError` with `Content-Type: application/json`.

| Endpoint | Success | Validation/errors |
|---|---|---|
| `POST /players` | 201: `{username, created_at}` | Username must match `[A-Za-z0-9_-]{1,32}` (400); case-insensitive duplicate (409). |
| `POST /scores` | 201: `{username, score, created_at}` | Require nonempty username and nonnegative integer score (400); unknown player (404). Use `Score *int64` to reject missing/null while allowing zero. |
| `GET /leaderboard?limit=10&offset=0` | 200: `{"leaderboard":[{"username":"player123","score":1500}],"total":1}` | Default limit 10; integer 1–100, otherwise 400. Offset default 0; non-negative integer, otherwise 400. Return each player's maximum score, ordered score DESC then username ASC; exclude unscored players; empty list is `[]`. `total` counts ranked players, read in the same transaction as the page. |
| `GET /health` | 200: `{"status":"ok"}` | |
| `GET /` | 200: web UI (HTML) | Exact path only; other unknown paths stay 404. |

POST bodies contain the named fields except `created_at`, which is server-generated UTC.

## Storage

- `players`: `id INTEGER PRIMARY KEY`, `username TEXT NOT NULL UNIQUE COLLATE NOCASE`, `created_at TEXT NOT NULL`.
- `scores`: `id INTEGER PRIMARY KEY`, `player_id INTEGER NOT NULL REFERENCES players(id)`, `score INTEGER NOT NULL CHECK (score >= 0)`, `created_at TEXT NOT NULL`.
- Create tables/index idempotently; index `idx_scores_player_score` on `scores(player_id, score)`.
- Set `foreign_keys=ON`, `journal_mode=WAL`, `busy_timeout=5000`, and `db.SetMaxOpenConns(1)`.
- Insert players directly; map UNIQUE violations to `store.ErrUsernameTaken`.
- Insert scores atomically with `INSERT INTO scores (player_id, score, created_at) SELECT id, ?, ? FROM players WHERE username = ?`; zero affected rows means `store.ErrPlayerNotFound`.
- Parameterize queries and pass `r.Context()`. Log internal errors with `log/slog`; return 500 without exposing SQL errors.

## Runtime

- `PORT=8080`, `DATABASE_PATH=./game.db` by default.
- Set `ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`, and `IdleTimeout`.
- Shut down gracefully using `signal.NotifyContext` for SIGINT/SIGTERM.

## Verification & Documentation

Before finishing any change, run `gofmt -l .` (no output), `go vet ./...`, and `go test -race ./...`.
Cover validation edge cases, duplicates, unknown players, maximum-score leaderboard, invalid limits and offsets, paging, the UI route, and concurrent requests. Check UI changes by hand in a browser. Never use `:memory:` for tests.

- Commits (conventional commits + gitmoji): `type(scope): <emoji> <summary>`, max 50 characters. 
- Architecture, schema, contract, or deployment changes require a Nygard ADR in `docs/adr/`.
- README: run/test commands, Postman/Newman and curl instructions, ADR links, limitations, and future improvements.
- Keep `postman/mini-game-api.postman_collection.json` in sync with the API contract: every endpoint and error case gets a request with `pm.test` checks. After an API change, run it with `npx newman run` against a live server.
