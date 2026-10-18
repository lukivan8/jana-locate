"""Offline numeric-only service. No libpostal, transformer, reference DB or cache."""
import json,os,hashlib
from pathlib import Path
from http.server import BaseHTTPRequestHandler,HTTPServer
import pycrfsuite
from numeric_recovery import predict
model=Path(os.environ.get('NUMERIC_MODEL_PATH','/app/model.crfsuite'))
expected='1cf4bdc47fbac61c18ebcedc78bd19022eacaff799a38d1bdc75b886c29dabd4'
if hashlib.sha256(model.read_bytes()).hexdigest()!=expected:raise RuntimeError('numeric model checksum mismatch')
tagger=pycrfsuite.Tagger();tagger.open(str(model))
class Handler(BaseHTTPRequestHandler):
 def log_message(self,*args):pass
 def setup(self):
  super().setup();self.connection.settimeout(15)
 def send(self,status,obj):
  data=json.dumps(obj,ensure_ascii=False).encode()
  try:
   self.send_response(status);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(data)));self.end_headers();self.wfile.write(data)
  except (BrokenPipeError,ConnectionResetError):pass
 def do_GET(self):self.send(200,{'status':'ready','model_sha256':expected}) if self.path=='/ready' else self.send(404,{'error':'not_found'})
 def do_POST(self):
  if self.path!='/numeric':return self.send(404,{'error':'not_found'})
  try:
   n=int(self.headers.get('Content-Length','0'))
   if not 0<n<=16*1024*1024:raise ValueError()
   rows=json.loads(self.rfile.read(n))['records']
   if not isinstance(rows,list) or not 1<=len(rows)<=1000:raise ValueError()
   if any(not isinstance(r,dict) or not isinstance(r.get('raw_address'),str) or len(r['raw_address'])>4096 or 'id' not in r for r in rows):raise ValueError()
  except (ValueError,KeyError,TypeError):return self.send(400,{'error':'invalid_input'})
  results=[dict(id=r['id'],raw_address=r['raw_address'],proposals=predict(tagger,r)) for r in rows]
  self.send(200,{'results':results})
HTTPServer(('0.0.0.0',8080),Handler).serve_forever()
