# Verification lifecycle

`scripts/verify-all.sh` keeps its historical independent mode by default. To run
compatible stack verifiers against one runner-owned API/Postgres stack, set
`VERIFY_ALL_SHARED_STACK=1`. The runner chooses a unique Compose project and
ports, builds and starts the API once, exports the explicit `VERIFY_SHARED_*`
context, and removes only that project's containers, network, and volume on every
exit path.

The `insights` and `pricing` verifiers opt into this context. Their standalone
invocations still own their own stack and cleanup. CLI-only, control, and
configuration-specific verifiers remain independent because their fixtures or
build controls are not compatible with this shared context.
