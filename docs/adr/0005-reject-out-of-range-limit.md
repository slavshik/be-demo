# 5. Reject out-of-range leaderboard limit

Date: 2026-10-20

## Status

Accepted

Amended by [7. Web UI and paged leaderboard](0007-web-ui-and-paged-leaderboard.md)

## Context

`GET /leaderboard` needs a default `limit` and validation. Bad values can
either be clamped into range or rejected.

## Decision

Default `limit` is 10. Accept integers from 1 to 100. Anything else,
including non-integers, `0` and `101`, returns `400`.

## Consequences

Client bugs surface immediately instead of quietly returning a different
number of rows. Clients that want more than 100 entries need pagination,
which is out of scope.
