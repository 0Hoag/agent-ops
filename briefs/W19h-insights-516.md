# Insights verifier invocation

Run the verifier directly from the repository root. It serializes every
Docker/Compose operation through the canonical heavy runner internally.

```bash
bash scripts/verify/insights.sh
INSIGHTS_VERIFY_CONTROL=1 bash scripts/verify/insights.sh
```

Do not wrap either command in `scripts/heavy.sh`: the heavy lock is global and
non-reentrant, so an outer wrapper would wait for the verifier's inner heavy
operation while the verifier waits for the outer lock.

The control mode mutates only a temporary API source copy, uses the identical
HTTP request as the healthy run, requires the cache-hit-rate arithmetic
assertion to fail, and then relies on cleanup to remove its isolated project.
