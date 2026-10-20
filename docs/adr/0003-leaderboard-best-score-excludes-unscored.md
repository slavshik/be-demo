# 3. Leaderboard ranks best score and excludes unscored players

Date: 2026-10-20

## Status

Accepted

## Context

The spec ranks players by highest submitted score and asks to "handle
players who have not submitted any scores". Unscored players could be left
out, listed with `null`, or listed with `0`.

## Decision

Each player appears once with their `MAX(score)`. Order is score
descending, then username ascending. Players with no scores are left out
(an inner join).

## Consequences

Ties have a stable order and the response never contains `null` scores.
A frontend can't show registered-but-unscored players from this endpoint.
Listing them with `0` would make them indistinguishable from a real
zero score.
