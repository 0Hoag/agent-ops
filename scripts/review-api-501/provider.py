import http.server,json
class Handler(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  print(self.path,flush=True)
  if self.headers.get('PRIVATE-TOKEN')!='review-provider-token': self.send_error(401);return
  p=self.path.split('?')[0]
  if p.endswith('/issues'):v=[{'id':1,'iid':1,'title':'Synthetic issue','state':'opened','labels':[],'created_at':'2026-11-12T00:00:00Z','updated_at':'2026-11-12T00:00:00Z'}]
  elif p.endswith('/merge_requests') or p.endswith('/pipelines'):v=[]
  elif p.startswith('/api/v4/projects/'):v={'id':1,'name':'Synthetic review'}
  else:self.send_error(404);return
  raw=json.dumps(v).encode();self.send_response(200);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(raw)));self.end_headers();self.wfile.write(raw)
http.server.HTTPServer(('0.0.0.0',35082),Handler).serve_forever()
