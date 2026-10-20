# 4. Case-insensitive, restricted usernames

Date: 2026-10-20

## Status

Accepted

## Context

The spec only requires usernames to be non-empty and unique. Without more
rules, `Player` and `player` could both exist, and names could hold
whitespace or control characters that render badly in the game UI.

## Decision

Trim whitespace, then require 1–32 characters from `[A-Za-z0-9_-]`.
Enforce uniqueness in the database with `UNIQUE COLLATE NOCASE` and map the
constraint error to `409`, rather than checking before inserting.

## Consequences

Look-alike names are impossible and duplicate checks are race-free.
Non-ASCII names are rejected; supporting them would need Unicode
normalization.
