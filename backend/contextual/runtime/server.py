import hashlib,json,pathlib,sys,time,torch,subprocess,threading,signal
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
ROOT=pathlib.Path('/lab/goal');sys.path[:0]=[str(ROOT/'contextual'),str(ROOT/'reference-candidates'),str(ROOT/'reference-transliteration')]
from encoder import Encoder
from head_joint import make_head
from predict_sequence_joint import predict
from resolver import Resolver
from tiered_lookup import TieredLookup
sys.path.insert(0,str(ROOT/'integrated-candidate-v1'))
from diagnostics import numeric_evidence
sys.path.insert(0,str(ROOT/'numeric-ensemble-v1'))
from fusion_grounded import fuse
sys.path.insert(0,str(ROOT/"numeric-ambiguity-v1"))
from diagnose import diagnose
sys.path.insert(0,str(ROOT/"settlement-alias-overlay-v1"))
from overlay import Overlay
sys.path.insert(0,str(ROOT/"district-presentation-v1"))
from adapter import apply as present_district
sys.path.insert(0,str(ROOT/"microdistrict-specialist-v1"))
from stage import apply_after_fusion
sys.path.insert(0,str(ROOT/"parse-branch-references-v1"))
from branches import collect as collect_branches
from full_lookup import FullLookup

started=time.monotonic();
import importlib.util
spec=importlib.util.spec_from_file_location('unified_reference_stage',ROOT/'unified-reference-v1/adapter.py');unified=importlib.util.module_from_spec(spec);spec.loader.exec_module(unified)
sys.path.insert(0,str(ROOT/'reference-compatibility-v1'))
from graph import graph as compatibility_graph
reference_stage=unified.ReferenceStage()
torch.set_num_threads(2);torch.set_num_interop_threads(1)
net=make_head();net.load_state_dict(torch.load(ROOT/'contextual/head-v7/model.pt',weights_only=True));net.eval();encoder=Encoder(ROOT/'contextual/assets')
specialist_sha=hashlib.sha256((ROOT/'microdistrict-alphanumeric-v1/trained/model.pt').read_bytes()).hexdigest()
specialist=make_head();specialist.load_state_dict(torch.load(ROOT/'microdistrict-alphanumeric-v1/trained/model.pt',weights_only=True));specialist.eval()
def normalize(rows):
 result=[];timings={k:0.0 for k in ['native','encoder','head_and_fusion','specialist_head','reference','diagnostics']};stage=time.monotonic()
 native=subprocess.run(['/lab-trace'],input=('\0'.join(r['raw_address'].replace('\0',' ') for r in rows)+'\0').encode(),capture_output=True,check=True,timeout=120)
 timings['native']+=time.monotonic()-stage
 parts=[json.loads(s) for s in native.stdout.decode().splitlines()]
 if len(parts)!=len(rows):raise RuntimeError('native_output_count')
 # Bounded batches; features are freshly computed and never loaded from eval files.
 for offset in range(0,len(rows),32):
  batch=rows[offset:offset+32];stage=time.monotonic();features=encoder.encode([r['raw_address'] for r in batch]);timings['encoder']+=time.monotonic()-stage
  for j,(row,feat) in enumerate(zip(batch,features)):
   stage=time.monotonic();pred=fuse(predict(row,feat,net),parts[offset+j]);timings['head_and_fusion']+=time.monotonic()-stage;stage=time.monotonic()
   proposal=fuse(predict(row,feat,specialist),parts[offset+j]);pred=apply_after_fusion(pred,proposal)
   if pred.get('microdistrict_alternative'):pred['microdistrict_alternative'].update(source='microdistrict-alphanumeric-v1',model_sha256=specialist_sha,arbitration_policy='microdistrict-specialist-v1')
   timings['specialist_head']+=time.monotonic()-stage;stage=time.monotonic();pred=reference_stage.apply(pred);pred['reference_compatibility']=compatibility_graph(pred,reference_stage.entities);warnings=list(pred['warnings'])
   timings['reference']+=time.monotonic()-stage;stage=time.monotonic()
   numeric=numeric_evidence(pred);pred['numeric_evidence']=numeric['numeric_spans'];warnings+=numeric['warnings'];pred['warnings']=sorted(set(warnings));pred['status']='experimental_candidate';pred['confidence']=None
   pred=present_district(diagnose(pred));timings["diagnostics"]+=time.monotonic()-stage
   result.append(pred)
 return result,timings
inference_lock=threading.Lock()
inference_slots=threading.BoundedSemaphore(4)
startup_seconds=time.monotonic()-started
class Handler(BaseHTTPRequestHandler):
 def setup(self):
  super().setup();self.connection.settimeout(300)
 def log_message(self,*args):pass
 def send(self,status,obj):
  body=json.dumps(obj,ensure_ascii=False).encode()
  try:
   self.send_response(status);self.send_header('Content-Type','application/json');self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
  except (BrokenPipeError,ConnectionResetError):pass
 def do_GET(self):self.send(200,{'status':'ready','startup_seconds':startup_seconds}) if self.path=='/ready' else self.send(404,{'error':'not_found'})
 def do_POST(self):
  if self.path!='/normalize':return self.send(404,{'error':'not_found'})
  try:
   n=int(self.headers.get('Content-Length','0'))
   if not 0<n<=4*1024*1024:raise ValueError('payload_size')
   rows=json.loads(self.rfile.read(n))['records']
   if not isinstance(rows,list) or not 1<=len(rows)<=1000:raise ValueError('batch_size')
   if any(not isinstance(r,dict) or not isinstance(r.get('raw_address'),str) or len(r['raw_address'])>4096 or 'id' not in r for r in rows):raise ValueError('record_shape')
  except (ValueError,KeyError,TypeError) as e:return self.send(400,{'error':str(e)})
  if not inference_slots.acquire(blocking=False):return self.send(503,{'error':'inference_busy'})
  if not inference_lock.acquire(timeout=150):
   inference_slots.release();return self.send(503,{'error':'inference_busy'})
  try:
   t=time.monotonic();out,timings=normalize(rows);self.send(200,{'results':out,'processing_seconds':time.monotonic()-t,'stage_seconds':timings})
  except (BrokenPipeError,ConnectionResetError):pass
  except Exception as e:
   print(json.dumps({'event':'inference_failed','type':type(e).__name__}),flush=True)
   self.send(503,{'error':'inference_failed'})
  finally:
   inference_lock.release();inference_slots.release()
print(json.dumps({'event':'ready','startup_seconds':startup_seconds}),flush=True)
server=ThreadingHTTPServer(('0.0.0.0',8080),Handler)
signal.signal(signal.SIGTERM,lambda *_: threading.Thread(target=server.shutdown,daemon=True).start())
server.serve_forever()
server.server_close()
