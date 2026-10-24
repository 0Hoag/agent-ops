# W1cr11 acceptance evidence

Measured on 2026-10-24 at commit `9415dbc1e8475184040e06514370beea4fe690f7`.

## Isolated signed HTTP/PostgreSQL smoke

The historical disposable fixture was `postgres:17-alpine`, container
`w1cr10-postgres`, published on `127.0.0.1:55434`, with database/user `smoke`.
The API was built from that checkout and started on `127.0.0.1:18193`.
The script used two configured repositories (`0Hoag/agent-ops` and
`example/other`) and local-only secrets. Readiness was polled with
`curl http://127.0.0.1:18193/readyz` before requests.

Command:

```sh
zsh .agent-runs/w1cr10-smoke.sh
```

That ignored historical script is not available from a clean checkout. For a reproducible
current run, use the committed command documented in [W1e signed ingestion smoke](w1e-ingestion-smoke.md).

The script's HMAC extraction uses the portable `openssl dgst -hex` output. The
prior attempt used field 2 of OpenSSL output, which is empty on this host and
explains its false all-400 result. The corrected run produced:

```text
READY_SHA=9415dbc1e8475184040e06514370beea4fe690f7
A_A=200 {"accepted":true}
AFTER_A=1|1|0Hoag/agent-ops:910:closed
B_A=400 {"message":"webhook rejected","status":400}
AFTER_B=1|1|0Hoag/agent-ops:910:closed
SIGNED_UNKNOWN=400 {"message":"webhook rejected","status":400}
AFTER_UNKNOWN=1|1|0Hoag/agent-ops:910:closed
DUPLICATE=200 {"accepted":false}
AFTER_DUPLICATE=1|1|0Hoag/agent-ops:910:closed
```

The final identity query returned exactly one row: repository
`0Hoag/agent-ops`, delivery `w1cr10-a-910`, issue `910`, state `closed`.
The historical trap stopped the API but did **not** remove its container or
anonymous volume. The container was found still healthy after the run; the
coordinator later removed that owned fixture and volume manually. Other running
services were left untouched. This correction supersedes the previous cleanup
claim; see the committed W1e smoke for verified current cleanup behavior.

## Controls and gates

The focused committed test passed:

```sh
cd api && go test ./modules/webhook -run TestControllerHandleBindsSignedPayloadToConfiguredRepository -count=1 -v
```

It covers matching A/A persistence, B/A rejection, unknown-repository
rejection, explicit transaction-begin access assertions, and HMAC verification.
The retained disposable controls in the prior evidence removed both
repository-binding guards and bypassed GitHub HMAC verification; the focused
test failed with explicit unexpected database-access diagnostics, and source
was restored.

Fresh local results from `api/` were successful for `go vet ./...`,
`go test ./...`, and `go build ./...`. The installed local `golangci-lint`
2.11.4 reported `context loading failed: no go files to analyze` despite
`go list ./...` returning all packages; the hosted API check passed on the
exact head in workflow run
https://github.com/0Hoag/agent-ops/actions/runs/40042848499.
