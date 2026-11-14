# Sourced by control.sh: same stack, same stored wave, same PATCH, normal HTTP oracle.
python3 "$ROOT/scripts/verify/control/acceptance.py" "http://127.0.0.1:$PORT" "$ARTIFACT_DIR" prepare > "$ARTIFACT_DIR/mutation-prepare.log"
sql 'CREATE TABLE control_wave_snapshot AS SELECT * FROM waves;' >/dev/null
probe() { python3 "$ROOT/scripts/verify/control/acceptance.py" "http://127.0.0.1:$PORT" "$ARTIFACT_DIR" probe; }
probe > "$ARTIFACT_DIR/control-before.log"
cp -R "$RUN/api" "$RUN/mutant"
python3 - "$RUN/mutant/modules/wave/models.go" <<'PY'
from pathlib import Path
import sys
p = Path(sys.argv[1]); s = p.read_text()
old = 'return allowed[from][to]'
assert s.count(old) == 1, 'mutation target missing'
p.write_text(s.replace(old, 'return allowed[from][to] || (from == Todo && to == Done)'))
PY
cat > "$RUN/mutation.yml" <<YAML
services:
  api:
    image: $PROJECT-api:mutation
    build: $RUN/mutant
YAML
mutant() { "$HEAVY" docker compose -p "$PROJECT" -f "$RUN/compose.yml" -f "$RUN/mutation.yml" "$@"; }
ready() {
  for _ in $(seq 1 90); do curl -fsS "http://127.0.0.1:$PORT/readyz" >/dev/null 2>&1 && return; sleep 1; done
  echo 'FAIL: API readiness'; return 1
}
mutant build api > "$ARTIFACT_DIR/mutation-build.log" 2>&1
mutant up -d --no-build api > "$ARTIFACT_DIR/mutation-start.log" 2>&1
ready
if probe > "$ARTIFACT_DIR/control-failing.log" 2>&1; then
  echo 'FAIL: production mutation unexpectedly passed'; exit 1
fi
grep -q 'AssertionError: WAVE_INVALID_TRANSITION_REJECTED: HTTP_STATUS PATCH .*expected HTTP 400, actual 200' "$ARTIFACT_DIR/control-failing.log"
echo 'PASS: mutation fails at WAVE_INVALID_TRANSITION_REJECTED (expected 400, actual 200)'
# Restore the complete stored row before sending the identical request to the original image.
sql 'UPDATE waves w SET state=s.state, updated_at=s.updated_at FROM control_wave_snapshot s WHERE w.id=s.id;' >/dev/null
compose up -d --no-build api > "$ARTIFACT_DIR/restore-start.log" 2>&1
ready
probe > "$ARTIFACT_DIR/control-restored.log"
echo 'PASS: original image restored; identical stored wave and PATCH pass normal oracle'
