# 7. Web UI and paged leaderboard

Date: 2026-10-20

## Status

Accepted

Amends [5. Reject out-of-range leaderboard limit](0005-reject-out-of-range-limit.md)

## Context

We want a simple browser UI to add players, submit scores, and page through
the leaderboard. ADR 5 capped `limit` at 100 and left pagination out of
scope, so a client could never see past the top 100, and it had no way to
know how many pages exist.

## Decision

`GET /leaderboard` takes an optional `offset` (non-negative integer, default
0; anything else is `400`) and its response gains `total`, the number of
ranked players. The page and the total are read in one transaction so they
agree. The `limit` rules from ADR 5 are unchanged.

The UI is one static `index.html` with plain JavaScript and no build step.
It lives in `internal/web`, is compiled into the binary with `embed`, and is
served at `GET /` (exactly; other paths still return `404`). It calls the
same JSON API as any other client, from the same origin, so no CORS is
needed. It includes a score form because unscored players are not on the
leaderboard (ADR 3).

## Consequences

Existing clients are unaffected: `offset` is optional and `total` is an extra
field. Offset paging is simple but can skip or repeat a row if scores change
between page loads; keyset pagination would fix that if it matters. The UI
adds a package and a non-Go file to maintain; the Go tests check that it is
served, and its own behaviour is checked by hand in a browser.
