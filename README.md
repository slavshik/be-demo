# Mini Game Backend API

A REST API for a multiplayer game. Players register, submit scores, and read
a leaderboard. It's built on Go's standard library with SQLite.

## Quick start

Requires Go 1.26+ (see `go.mod`). No CGO or system SQLite needed.

```sh
make run                    # listens on :8080, creates ./game.db
make test                  # formatting, vet, race tests
```

### Command interface

Requires Make. Four commands cover the workflow:

| Command | Purpose |
|---------|---------|
| `make build` (or `make`) | Build `./server` |
| `make run` | Run from source |
| `make test` | Check formatting, run vet and race tests |
| `make deploy` | Request a Render deployment (requires curl and a deploy hook) |

Override runtime settings with `make run PORT=9090 DATABASE_PATH=./local.db`.
For Postman/Newman and curl commands, see [Testing](#testing).

| Env var         | Default     |
|-----------------|-------------|
| `PORT`          | `8080`      |
| `DATABASE_PATH` | `./game.db` |

The schema is created on startup. Ctrl+C stops the server after in-flight
requests finish.

## API

All bodies are JSON. Errors look like `{"error": "message"}`.

| Endpoint                    | Success | Errors                                    |
|-----------------------------|---------|-------------------------------------------|
| `POST /players`             | `201`   | `400` invalid username, `409` taken       |
| `POST /scores`              | `201`   | `400` invalid input, `404` unknown player |
| `GET /leaderboard?limit=10` | `200`   | `400` invalid limit                       |
| `GET /health`               | `200`   |                                           |

- **Usernames** are trimmed, 1–32 characters from `A-Z a-z 0-9 _ -`, and
  unique regardless of case.
- **Scores** are non-negative integers. Every submission is stored with a
  server-side UTC timestamp.
- **Leaderboard** shows each player's best score, highest first. Ties are
  ordered by username, and players with no scores are left out. `limit`
  defaults to 10 and must be 1–100.

```
POST /players  {"username": "player123"}
→ 201 {"username": "player123", "created_at": "2026-10-20T12:15:06.469733Z"}

POST /scores   {"username": "player123", "score": 1500}
→ 201 {"username": "player123", "score": 1500, "created_at": "2026-10-20T12:15:06.496796Z"}

GET /leaderboard?limit=2
→ 200 {"leaderboard": [{"username": "player123", "score": 1500},
                       {"username": "player456", "score": 1200}]}
```

## Testing

There are two layers of tests:

- **Go tests** (`go test -race ./...`) exercise the handler directly against
  a fresh SQLite file per test. They cover every validation edge case, and
  include a concurrency test: of 20 simultaneous registrations of one name,
  exactly one gets `201`.
- **The Postman collection** checks a real running server over HTTP.

### Postman

[`postman/mini-game-api.postman_collection.json`](postman/mini-game-api.postman_collection.json)
has 19 requests in four groups (Health, Players, Scores, Leaderboard). Each
request has tests that check its status code and response body.

1. Start the server: `go run ./cmd/server`.
2. In Postman, choose **Import** and select the collection file.
3. Open the collection, choose **Run**, and run it.

Run the requests in order. "Register player" generates a fresh username
(`player_<timestamp>`) that later requests reuse, so you can run the
collection any number of times against the same database. To send single
requests by hand, run "Register player" first.

The server URL is the `baseUrl` collection variable (default
`http://localhost:8080`). Change it in the collection's **Variables** tab to
test another port or a deployed instance.

### Newman (Postman from the command line)

Newman runs the same collection without the Postman app. It needs Node.js:

```sh
npx newman run postman/mini-game-api.postman_collection.json

# Against another server
npx newman run postman/mini-game-api.postman_collection.json \
  --env-var baseUrl=https://your-app.onrender.com
```

### curl

For a quick check without Postman:

```sh
curl -i -X POST localhost:8080/players -d '{"username":"player123"}'           # 201
curl -i -X POST localhost:8080/players -d '{"username":"PLAYER123"}'           # 409
curl -i -X POST localhost:8080/scores  -d '{"username":"player123","score":1500}' # 201
curl -i -X POST localhost:8080/scores  -d '{"username":"ghost","score":10}'     # 404
curl -i 'localhost:8080/leaderboard?limit=3'                                    # 200
curl -i 'localhost:8080/leaderboard?limit=0'                                    # 400
```

## Project layout

```
cmd/server/main.go        config, HTTP server, graceful shutdown
internal/store/store.go   SQLite schema and queries
internal/api/handlers.go  routes, validation, JSON responses
internal/api/api_test.go  Go tests
postman/                  Postman collection
docs/adr/                 architecture decision records
```

## Design choices

Each decision is recorded in [`docs/adr/`](docs/adr/README.md).

- **Go standard library + SQLite**
  ([ADR 2](docs/adr/0002-go-stdlib-http-and-sqlite.md)). `net/http` routing
  covers four routes. SQLite gives real persistence with no setup, and the
  pure-Go driver needs no CGO.
- **Best score per player, unscored players left out**
  ([ADR 3](docs/adr/0003-leaderboard-best-score-excludes-unscored.md)).
- **Case-insensitive, restricted usernames**
  ([ADR 4](docs/adr/0004-case-insensitive-usernames.md)). The database
  enforces uniqueness, so two concurrent registrations can't both succeed.
- **Out-of-range `limit` is rejected, not clamped**
  ([ADR 5](docs/adr/0005-reject-out-of-range-limit.md)).
- **Render as the deployment target**
  ([ADR 6](docs/adr/0006-deploy-to-render.md)).

## Deploy (Render)

Create a Render web service from this repository:

| Setting           | Value                             |
|-------------------|-----------------------------------|
| Build command     | `make build`                     |
| Start command     | `./server`                        |
| Health check path | `/health`                         |

Render sets `PORT`. On the free tier the disk is ephemeral, so the SQLite
file is lost on every redeploy or restart. To keep data, mount a persistent
disk and point `DATABASE_PATH` at it.

For manual deployments, copy the service's secret
[deploy hook URL](https://render.com/docs/deploy-hooks) from Render Settings
into the `RENDER_DEPLOY_HOOK_URL` environment variable, then run:

```sh
make test
git push origin main
make deploy
```

Commit changes before pushing. `make deploy` deploys the latest commit on
the branch configured in Render; it does not upload local files, commit,
or push. It requests deployment and returns without waiting for completion.
Check progress in Render, then run `curl --fail https://your-app.onrender.com/health`.
Keep the hook URL out of source control. If Render auto-deploy is enabled,
pushing already triggers deployment; the manual hook is optional.

## Limitations and next steps

- **No authentication.** Anyone can submit any score for any player. Next
  steps: per-player tokens, and scores validated against server-side game
  sessions to stop cheating.
- **No rate limiting.**
- **One SQLite writer.** That's fine for one instance; running several means
  moving to PostgreSQL.
- **The leaderboard is computed on every request.** At scale, keep a
  `best_score` column updated on insert, or use a Redis sorted set for
  O(log N) updates.
- **No pagination** beyond `limit`, and no endpoint for a player's score
  history.
- **No OpenAPI spec.**
