"""Real HTTP and RFC6455 client; standard library only, no API internals."""
import base64
import hashlib
import json
import os
from pathlib import Path
import socket
import struct
import sys
import time
import urllib.error
import urllib.request

BASE, directory = sys.argv[1:3]
OUT = Path(directory)
ORG = '51800000-0000-4000-8000-00000000000'


def check(condition, name):
    if not condition:
        raise AssertionError(name)
    print('PASS: ' + name, flush=True)


def http(method, path, token='', body=None, expected=200):
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(BASE + '/api/v1/' + path, data=data, method=method,
                                 headers={'Content-Type': 'application/json',
                                          'Authorization': 'Bearer ' + token})
    try:
        response = urllib.request.urlopen(req, timeout=10)
    except urllib.error.HTTPError as error:
        response = error
    raw = response.read()
    check(response.code == expected,
          f'HTTP_STATUS {method} {path}: expected HTTP {expected}, actual {response.code}')
    return json.loads(raw) if raw else None


def login(who):
    token = http('POST', 'auth/login', body={'email': who + '@control.invalid',
                 'password': 'control-518-password'})['token']
    return http('POST', 'auth/select-organization', token,
                {'organization_id': ORG + ('2' if who == 'foreign' else '1')})['token']


def websocket(job, token, expected=101):
    port = int(BASE.rsplit(':', 1)[1])
    sock = socket.create_connection(('127.0.0.1', port), timeout=10)
    key = base64.b64encode(os.urandom(16)).decode()
    sock.sendall((f'GET /api/v1/workers/{job}/logs HTTP/1.1\r\nHost: 127.0.0.1:{port}\r\n'
                  'Upgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Version: 13\r\n'
                  f'Sec-WebSocket-Key: {key}\r\nAuthorization: Bearer {token}\r\n\r\n').encode())
    reader = sock.makefile('rb')
    status = int(reader.readline().split()[1])
    headers = {}
    while True:
        line = reader.readline().decode().strip()
        if not line:
            break
        k, v = line.split(':', 1)
        headers[k.lower()] = v.strip()
    check(status == expected, f'WebSocket {job}: expected {expected}, actual {status}')
    if expected != 101:
        reader.close()
        sock.close()
        return None
    accept = base64.b64encode(hashlib.sha1((key + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').encode()).digest()).decode()
    check(headers.get('sec-websocket-accept') == accept, 'RFC6455 handshake')
    return sock, reader


def frame(stream):
    _, reader = stream
    header = reader.read(2)
    check(len(header) == 2 and header[0] == 0x81, 'WebSocket text frame')
    size = header[1] & 127
    if size == 126:
        size = struct.unpack('!H', reader.read(2))[0]
    elif size == 127:
        size = struct.unpack('!Q', reader.read(8))[0]
    return json.loads(reader.read(size))


def logs(stream, count):
    messages = [frame(stream) for _ in range(count)]
    check(messages == [{'type': 'worker.log', 'line': f'stub:{i:02}'}
                       for i in range(1, count + 1)], 'WS_LOG_COMPLETENESS_ORDER')
    return messages


def close(stream):
    stream[1].close()
    stream[0].close()


def state(job, wanted):
    for _ in range(100):
        result = http('GET', 'workers/' + job, owner)['data']['State']
        if result == wanted:
            check(True, 'worker state=' + wanted)
            return
        time.sleep(.1)
    check(False, f'worker state expected {wanted}, actual {result}')


owner = login('owner')
if len(sys.argv) > 3 and sys.argv[3] == 'prepare':
    fixture = http('POST', 'waves', owner, {'name': 'mutation', 'brief': 'identical inputs'}, expected=201)['data']
    fixture = http('GET', 'waves/' + fixture['id'], owner)['data']
    (OUT / 'mutation-fixture.json').write_text(json.dumps(fixture, sort_keys=True))
    sys.exit(0)
if len(sys.argv) > 3 and sys.argv[3] == 'probe':
    fixture = json.loads((OUT / 'mutation-fixture.json').read_text())
    # Use the same normal HTTP status oracle as lifecycle acceptance.
    try:
        http('PATCH', 'waves/' + fixture['id'], owner, {'state': 'done'}, expected=400)
    except AssertionError as error:
        raise AssertionError('WAVE_INVALID_TRANSITION_REJECTED: ' + str(error)) from error
    check(http('GET', 'waves/' + fixture['id'], owner)['data'] == fixture,
          'WAVE_INVALID_TRANSITION_UNCHANGED')
    sys.exit(0)
if len(sys.argv) > 3:
    before = json.loads((OUT / 'audit.json').read_text())
    after = http('GET', 'workers/audit-history?per_page=100', owner)
    check(after == before, 'audit identical after API restart (persisted Postgres history)')
    sys.exit(0)
member, foreign = login('member'), login('foreign')
wave = http('POST', 'waves', owner, {'name': 'W518', 'brief': 'harmless fixture'}, expected=201)['data']
check(wave['state'] == 'todo', 'wave initial state todo')
wid = wave['id']
http('GET', 'waves', expected=401)
check(http('GET', 'waves', foreign)['meta']['total'] == 0, 'foreign wave list empty')
check(http('GET', 'waves?per_page=1', owner)['data'][0]['id'] == wid, 'wave list pagination')
# The mutation changes only the production transition guard. Same request, same row.
http('PATCH', 'waves/' + wid, owner, {'state': 'done'}, expected=400)
check(http('GET', 'waves/' + wid, owner)['data']['state'] == 'todo',
      'WAVE_INVALID_TRANSITION_UNCHANGED')
for value in ['in_progress', 'blocked', 'in_progress', 'review', 'done']:
    check(http('PATCH', 'waves/' + wid, owner, {'state': value})['data']['state'] == value,
          'wave transition=' + value)
http('GET', 'waves/' + wid, foreign, expected=404)
http('PATCH', 'waves/' + wid, foreign, {'state': 'review'}, expected=404)
http('POST', 'waves', member, {'name': 'denied', 'brief': 'denied'}, expected=403)
request = {'Wave': 'W518', 'Branch': 'verify/hold', 'Brief': '/fixture/brief',
           'Worktree': '/fixture', 'Script': 'stub'}
http('POST', 'workers/start', member, request, expected=403)
job = http('POST', 'workers/start', owner, request, expected=201)['data']
check(job['State'] == 'queued', 'start queued snapshot')
jid = job['ID']
state(jid, 'running')
http('GET', 'workers/' + jid, foreign, expected=404)
http('POST', 'workers/' + jid + '/stop', foreign, expected=404)
websocket(jid, foreign, 404)
websocket(jid, member, 403)
stream = websocket(jid, owner)
messages = logs(stream, 3)
http('POST', 'workers/' + jid + '/stop', owner)
check(frame(stream) == {'type': 'worker.completed'}, 'cancelled stream completion')
close(stream)
state(jid, 'cancelled')
request['Branch'] = 'verify/finish'
restart = http('POST', 'workers/restart', owner, request, expected=201)['data']['ID']
check(restart != jid, 'restart creates distinct owned job')
stream = websocket(restart, owner)
messages += logs(stream, 5)
check(frame(stream) == {'type': 'worker.completed'}, 'replayed stream completion')
close(stream)
state(restart, 'succeeded')
stream = websocket(restart, owner)
logs(stream, 5)
check(frame(stream) == {'type': 'worker.completed'}, 'completed replay matches live stream')
close(stream)
(OUT / 'websocket.json').write_text(json.dumps(messages, indent=2))
audit = http('GET', 'workers/audit-history?per_page=100', owner)
rows = audit['data']
page = http('GET', 'workers/audit-history?per_page=1&page=2', owner)
check(page['meta']['total'] == len(rows) and page['data'] == rows[1:2], 'audit pagination order')
accepted = [(r['Action'], r['JobID']) for r in reversed(rows) if r['Outcome'] == 'accepted']
check(accepted == [('start', jid), ('stop', jid), ('restart', restart)],
      'audit exact accepted lifecycle order and job IDs')
check(all(r['OrganizationID'] == ORG + '1' and r['ActorID'] and r['OccurredAt'] for r in rows),
      'audit tenant, actor, timestamp populated')
check(sum(r['Outcome'] == 'role_rejected' for r in rows) == 2, 'role denials persisted')
check(http('GET', 'workers/audit-history', foreign)['meta']['total'] == 1,
      'foreign audit contains only its rejected stop')
(OUT / 'audit.json').write_text(json.dumps(audit, indent=2))
http('DELETE', 'waves/' + wid, owner, expected=204)
http('GET', 'waves/' + wid, owner, expected=404)
