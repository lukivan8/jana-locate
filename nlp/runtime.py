"""Local-only GLiNER CPU runtime. No network, GPU, model downloads or raw-address cache."""
import hashlib, json, os, threading
from pathlib import Path

# Must be set before importing transformers/HF. Missing assets fail startup.
os.environ['HF_HUB_OFFLINE']='1'
os.environ['TRANSFORMERS_OFFLINE']='1'
os.environ['HF_HUB_DISABLE_TELEMETRY']='1'
os.environ['TOKENIZERS_PARALLELISM']='false'
os.environ['CUDA_VISIBLE_DEVICES']=''

LABELS = {'province':'region', 'city':'city', 'district':'district',
          'living_area':'residential complex', 'street_name':'street',
          'house':'house number', 'apartment':'apartment number', 'entrance':'entrance number',
          'floor':'floor number', 'landmark':'landmark'}

class Runtime:
    def __init__(self, path, threads=2, microbatch=8, quantize=True):
        import torch
        from gliner import GLiNER, GLiNERConfig
        from transformers import AutoTokenizer
        from safetensors.torch import load_file
        path=Path(path).resolve()
        manifest=json.loads((path/'manifest.json').read_text())
        for filename,digest in manifest['files'].items():
            with (path/filename).open('rb') as file:
                if hashlib.file_digest(file,'sha256').hexdigest()!=digest:raise ValueError('Model checksum mismatch: '+filename)
        torch.set_num_threads(threads);torch.set_num_interop_threads(1)
        config=json.loads((path/'gliner_config.json').read_text())
        config['model_name']=str(path/'encoder')
        config=GLiNERConfig(**config)
        tokenizer=AutoTokenizer.from_pretrained(str(path/'encoder'),local_files_only=True)
        self.model=GLiNER(config,tokenizer=tokenizer,encoder_from_pretrained=False)
        if config.class_token_index==-1 or config.vocab_size==-1:
            self.model.resize_token_embeddings(add_tokens=['[FLERT]',config.ent_token,config.sep_token])
        self.model.model.load_state_dict(load_file(str(path/'model.safetensors'),device='cpu'),strict=True)
        self.model.to('cpu').eval()
        if quantize:
            # CPU int8 linear layers; embeddings remain float32. No accuracy claim
            # is made without evaluating this exact prepared model configuration.
            engines=torch.backends.quantized.supported_engines
            torch.backends.quantized.engine='x86' if 'x86' in engines else 'qnnpack' if 'qnnpack' in engines else 'fbgemm'
            self.model.model=torch.quantization.quantize_dynamic(self.model.model,{torch.nn.Linear},dtype=torch.qint8,inplace=True)
        self.lock=threading.Lock();self.microbatch=microbatch;self.manifest=manifest
        self.predict(['Астана, улица Туран 55/6'])  # warm up before readiness

    def predict(self,texts):
        import torch
        reverse={v:k for k,v in LABELS.items()}
        with self.lock,torch.inference_mode():
            rows=self.model.run(texts,list(LABELS.values()),threshold=0.25,batch_size=self.microbatch,flat_ner=False)
        return [[{'start':int(e['start']),'end':int(e['end']),'text':e['text'],
                  'label':reverse[e['label']],'score':float(e['score'])} for e in row] for row in rows]
