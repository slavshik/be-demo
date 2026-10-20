# 2. Go stdlib HTTP with SQLite storage

Date: 2026-10-20

## Status

Accepted

## Context

The challenge asks for a small REST API with three endpoints, built in
90 minutes, and accepts SQLite, PostgreSQL or in-memory storage. The author
must be able to explain every line.

## Decision

Use Go's `net/http` with method patterns in `http.ServeMux` (Go 1.22+), and
SQLite through `database/sql` with the pure-Go `modernc.org/sqlite` driver.
No web framework, ORM or migration tool. Schema is applied on startup with
`CREATE ... IF NOT EXISTS`.

## Consequences

Data survives restarts with zero setup and no CGO. The database enforces
uniqueness and `score >= 0`. A single SQLite writer limits horizontal
scaling; moving to PostgreSQL would be the next step. Any new third-party
dependency needs its own ADR.
