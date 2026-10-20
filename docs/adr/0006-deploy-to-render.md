# 6. Deploy to Render

Date: 2026-10-20

## Status

Accepted

## Context

The API should be reachable by the PixiJS frontend and by reviewers without
local setup. Render runs a Go web service from the repository with no
container setup, and its free tier has no persistent disk.

## Decision

Deploy as a Render web service. The server listens on `PORT`, reads the
SQLite file from `DATABASE_PATH`, and exposes `GET /health` for Render's
health check.

Use `make build` as the build command and `./server` as the start command.
The Makefile provides a manual `make deploy` command using Render's deploy
hook, supplied through the `RENDER_DEPLOY_HOOK_URL` environment variable.
It requests deployment of the configured branch without pushing local
changes or waiting for completion.

## Consequences

On the free tier the SQLite file is lost on every redeploy or restart. That
is acceptable for a demo; real use needs a persistent disk mounted at
`DATABASE_PATH`, or PostgreSQL (see ADR 2).
