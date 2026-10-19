# AGENTS.md

Go program (`github.com/alexhokl/domain-watch`) that checks domains over RDAP
and logs changes. Run-once: started by Cloud Scheduler as a Cloud Run job.

## Commands

- `task build`, `task test`, `task lint`, `task sec`, `task coverage`
- Single test: `go test ./... -run TestName -v`

Run `task lint` and `task test` before considering a change done.

## Architecture (single `main` package at repo root)

- `config.go` reads env vars (`DOMAINS`, `STATE_BUCKET`/`STATE_DIR`, ...).
- `rdap.go` looks a domain up: 404 means unregistered, otherwise the
  `expiration` event and `status[]`. Statuses are normalised (lower-case, no
  separators) because RDAP says "pending delete" while EPP says `pendingDelete`.
- `state.go` persists the previous result per domain (`FileStore`, `GCSStore`).
- `evaluate.go` is pure: previous state + current result + now -> events. All
  transition logic lives here and is table-tested.
- `logging.go` renames slog keys to Cloud Logging's `severity`/`message`/
  `timestamp`; the alert policy on GCP filters on `severity>=WARNING`.
- `main.go` wires it together. A lookup failure never overwrites saved state.

## Conventions

- Event names and severities are a contract with the alert policy on GCP; change
  both together.
- Dependencies must be pure Go (the image is `FROM scratch`, `CGO_ENABLED=0`).
- Releases: `task tag` then push the tag; Cloud Build publishes the image.
