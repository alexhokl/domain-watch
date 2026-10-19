# domain-watch

Checks domains over [RDAP](https://about.rdap.org/) and logs when one is
renewed, changes status, gets close to expiry, enters redemption or pending
delete, or is released. It runs once and exits; a scheduler (Cloud Run job plus
Cloud Scheduler) starts it daily, and a log-based alert emails on anything at
`WARNING` or above.

## Configuration

Environment variables:

| Variable       | Meaning                                                              |
| -------------- | -------------------------------------------------------------------- |
| `DOMAINS`      | Comma separated domains to check (required)                          |
| `STATE_BUCKET` | Cloud Storage bucket holding the previous result of each domain      |
| `STATE_DIR`    | Local directory for the same, for runs outside GCP (not with bucket) |
| `STATE_PREFIX` | Object/path prefix for state files (default `domain-watch`)          |
| `WARN_DAYS`    | Days before expiry that raise `expiring_soon` (default `60,30,7`)    |

Exactly one of `STATE_BUCKET` and `STATE_DIR` is required. The bucket is
accessed with Application Default Credentials.

## Local run

```bash
DOMAINS=example.com STATE_DIR=$(mktemp -d) go run .
```

## Events

Logs are JSON with Cloud Logging's `severity`, `message` and `timestamp` keys
and an `event` attribute:

| Event               | Severity | When                                                    |
| ------------------- | -------- | ------------------------------------------------------- |
| `first_check`       | NOTICE   | First time the domain is seen and nothing is wrong      |
| `no_change`         | NOTICE   | Nothing changed since the previous run                  |
| `expiry_changed`    | WARNING  | Expiry differs from last run (usually a renewal)        |
| `status_changed`    | WARNING  | Registry status set differs from last run               |
| `expiring_soon`     | WARNING  | Days left fell under one of `WARN_DAYS` (once each)     |
| `redemption_period` | WARNING  | Domain entered the redemption period                    |
| `domain_registered` | WARNING  | A previously available domain was registered            |
| `pending_delete`    | ERROR    | Domain is pending delete (every run while it is)        |
| `domain_available`  | ERROR    | RDAP says the domain is not registered                  |
| `check_failed`      | ERROR    | RDAP or storage failed; saved state is left untouched   |

A failed check makes the process exit non-zero.

