from http_common import *
import hashlib,hmac,uuid
payload={'repository':{'full_name':'review/github-fixture'},'issue':{'id':4,'number':4,'title':'GitHub signature fixture','state':'open','updated_at':'2026-11-12T00:00:00Z','labels':[]}}
for case in ('no-token','wrong-secret','missing-body','invalid-body','empty-data','valid'):
 body=b'' if case=='missing-body' else b'{' if case=='invalid-body' else b'{}' if case=='empty-data' else json.dumps(payload).encode()
 key=b'wrong-secret' if case=='wrong-secret' else b'review-secret'
 headers={'X-Github-Event':'issues','X-Github-Delivery':str(uuid.uuid4())}
 if case!='no-token':headers['X-Hub-Signature-256']='sha256='+hmac.new(key,body,hashlib.sha256).hexdigest()
 call('POST','/api/v1/webhooks/github',body,extra=headers,case=case)
(OUT/'github-auth-results.json').write_text(json.dumps(rows,indent=2))
assert [r['status'] for r in rows]==[400,400,400,400,400,200]
for row in rows:print(row['case'],row['status'],row['response'])
