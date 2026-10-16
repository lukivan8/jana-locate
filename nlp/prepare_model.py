"""Build-time only: fetch pinned weights AND encoder config/tokenizer, never encoder weights."""
import argparse, hashlib, json
from pathlib import Path
from huggingface_hub import snapshot_download

MODEL = 'urchade/gliner_multi-v2.1'
REVISION = '443d26d654e0324125a96bebd8e796c14ff2efe6'
ENCODER = 'microsoft/mdeberta-v3-base'
ENCODER_REVISION = 'a0484667b22365f84929a935b5e50a51f71f159d'

def prepare(path):
    path = Path(path).resolve()
    snapshot_download(MODEL, revision=REVISION, local_dir=path,
                      allow_patterns=['gliner_config.json', 'model.safetensors'])
    snapshot_download(ENCODER, revision=ENCODER_REVISION, local_dir=path / 'encoder',
                      allow_patterns=['config.json', 'tokenizer_config.json', 'tokenizer.json', 'spm.model', 'special_tokens_map.json'])
    files = {str(p.relative_to(path)): hashlib.sha256(p.read_bytes()).hexdigest()
             for p in path.rglob('*') if p.is_file() and '.cache' not in p.parts and p.name != 'manifest.json'}
    (path / 'manifest.json').write_text(json.dumps({'model': MODEL, 'revision': REVISION,
        'encoder_revision': ENCODER_REVISION, 'files': files}, indent=2) + '\n')
    print('Prepared pinned offline model:', path)

if __name__ == '__main__':
    parser = argparse.ArgumentParser(); parser.add_argument('--output', default='models/gliner')
    prepare(parser.parse_args().output)
