"""Render the archived HTTP measurements without credentials or repeated metrics text."""
from http_common import ROOT, OUT
import json

def redact(value):
    if isinstance(value, dict):
        return {key: ('[synthetic redacted]' if key in ('token', 'backfill_token', 'webhook_secret', 'link') else redact(item)) for key, item in value.items()}
    if isinstance(value, list):
        return [redact(item) for item in value]
    return value

def shape(value):
    if isinstance(value, dict):
        return '{' + ','.join(key + ':' + ('[]' if isinstance(item, list) else '{}' if isinstance(item, dict) else 'null' if item is None else 'n' if isinstance(item, (int, float)) else 's') for key, item in sorted(value.items())) + '}'
    return 'text' if isinstance(value, str) else '[]'

prefix = ROOT / 'docs/reviews/W18R1-api-501'
rows = [redact(row) for row in json.loads((OUT / 'final-http-results.json').read_text()) if row['case'] != 'setup']
for row in rows:
    if row['route'] == '/metrics':
        row['response'] = 'Prometheus text; full raw response retained in root archive.'
with open(str(prefix) + '-evidence.json', 'w') as target:
    json.dump(rows, target, indent=2)
for source, suffix in [('supplement-results.json', 'controls'), ('github-auth-results.json', 'github-auth')]:
    with open(str(prefix) + '-' + suffix + '.json', 'w') as target:
        json.dump(redact(json.loads((OUT / source).read_text())), target, indent=2)
with open(str(prefix) + '-matrix.md', 'w') as target:
    target.write('# API route matrix\n\nEach cell is HTTP status and response shape (`n` number, `s` string, `[]` array, `{}` object). Raw sanitized bodies: [evidence](W18R1-api-501-evidence.json). Supplemental defect/control requests: [controls](W18R1-api-501-controls.json). The valid webhook payload returns 400: defect #546, not a successful test.\n\n')
    target.write('| Method / route | No token | Other org | Missing body | Invalid body/query | Empty | Valid |\n|---|---|---|---|---|---|---|\n')
    for path, operations in json.loads((ROOT / 'api/docs/swagger.json').read_text())['paths'].items():
        for method in operations:
            measured = [row for row in rows if row['route'] == path and row['method'] == method.upper()]
            assert len(measured) == 6
            target.write('| ' + method.upper() + ' ' + path + ' | ' + ' | '.join(str(row['status']) + ' ' + ('upgrade' if row['status'] == 101 else shape(row['response'])) for row in measured) + ' |\n')
print('Rendered 63 operations with 378 route/case measurements')
