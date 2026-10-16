"""Transport/window tests use a stub, never stand in for real-model integration tests."""
import sys, unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from fastapi.testclient import TestClient
from app import app

class Stub:
    def __init__(self):self.windows=[]
    def predict(self,texts):
        self.windows.extend(texts)
        return [[{'start':i,'end':i+6,'text':'Алматы','label':'city','score':0.9}] if (i:=s.find('Алматы'))>=0 else [] for s in texts]
class Tests(unittest.TestCase):
    def setUp(self):
        self.stub=Stub();app.state.runtime=self.stub
        # No lifespan: model loading is checked separately with real weights.
        self.client=TestClient(app)
    def test_unicode_tail_and_input_order(self):
        text='x'*400+' Алматы'
        rows=self.client.post('/v1/entities',json={'texts':['',text]}).json()['results']
        self.assertEqual(rows[0],[]);self.assertEqual(len(rows[1]),1)
        e=rows[1][0];self.assertEqual(text[e['start']:e['end']],'Алматы')
        self.assertTrue(all(len(s)<=256 for s in self.stub.windows))
    def test_reject_invalid_and_oversized(self):
        self.assertEqual(self.client.post('/v1/entities',json={'texts':[]}).status_code,400)
        self.assertEqual(self.client.post('/v1/entities',json={'texts':['x'*4097]}).status_code,400)
        self.assertEqual(self.client.post('/v1/entities',content=b'x'*(2*1024*1024+1)).status_code,413)
if __name__=='__main__':unittest.main()
