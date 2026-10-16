"""Real-model startup/inference with outbound socket connections forbidden."""
import argparse,json,os,socket,tempfile,threading,time
from pathlib import Path
import psutil

p=argparse.ArgumentParser();p.add_argument('--model',required=True);p.add_argument('--output',required=True);a=p.parse_args()
process=psutil.Process();peak=[0];stop=threading.Event()
def sample():
    while not stop.wait(.02):peak[0]=max(peak[0],process.memory_info().rss)
thread=threading.Thread(target=sample);thread.start()
attempts=[]
def denied(self,address):
    attempts.append(str(address));raise RuntimeError('Network forbidden during offline verification')
socket.socket.connect=denied
with tempfile.TemporaryDirectory() as cache:
    os.environ['HF_HOME']=cache
    from runtime import Runtime
    start=time.monotonic();model=Runtime(a.model);startup=time.monotonic()-start
    texts=['Алматы, улица Новая 12а кв 4','Астана, проспект Туран 55/6','Караганда, 66квартал 12дом 69квартира']
    start=time.monotonic();rows=model.predict(texts);inference=time.monotonic()-start
stop.set();thread.join()
assert len(rows)==len(texts)
assert not attempts,attempts
report={'offline_startup_seconds':startup,'inference_seconds':inference,'records':len(texts),'device':'cpu',
        'quantized_linear_layers':True,'peak_sampled_rss_bytes':peak[0],'outbound_connection_attempts':attempts,
        'entities':rows,'scope':'Real pinned GLiNER weights, empty HuggingFace cache, sockets denied; no accuracy claim.'}
Path(a.output).write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report,ensure_ascii=False,indent=2))
