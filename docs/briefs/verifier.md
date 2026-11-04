# Verifier brief (tests the running system for real)

Use for acceptance of a user-visible feature or a whole area. Failing to find bugs after a long run is a reason to
doubt the run, not a result.

```
HAT         verifier for <feature/area> on <base URL / local compose>, repo <owner/repo>
BASE        <exact base>. Say which base every piece of evidence came from.
SCOPE       <the owner's boundary in the owner's words; usual: write only on records you create, named qc-<date>-<n>>.
            Nothing else is limited. Do not add your own limits ("read-only by default", "avoid side effects", "at most
            one"). List side effects in the report instead of avoiding them.
INVENTORY   FIRST write every function: screen, tab, button, form field and option, endpoint, state, role, error code.
            Test every row: do it for real, read the request the UI sent, read the result back with a fresh GET,
            look at the screenshot, then error path, empty state, reload.
DEPTH       empty / 1 / max / max+1, long and non-ASCII names, duplicates, double click, reload mid-operation, expired
            session, forced 4xx/5xx/429/503, every role. A state the account cannot reach: rewrite the response in the
            browser (`page.route`) and mark the row "mocked".
UI CLAIMS   drive the real UI and read the screenshot; API calls prove only the API.
NOT-TESTABLE only with an observed fact, quoted (e.g. the server answers 400 `url_blocked`). Caution, cost and "it would
            change data" are not facts.
HEAVY       browsers, docker compose and full builds go through `scripts/heavy.sh`.
NEVER       paste customer data; print a token or key.
REPORT      one row per function: WORKS / WORKS-DIFFERENTLY (how) / BROKEN (request, evidence) / NOT-TESTABLE (fact);
            tally; side effects; writes performed; cleanup proof; what was not covered.
```
