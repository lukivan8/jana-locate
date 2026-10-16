"""Offline real-model probe. Outputs may contain private input: keep them outside Git.

Compare independent processes with and without --quantize. Threshold/prompt probes
measure entity yield, not accuracy; use the Go ablation to measure field correctness.
"""
import argparse
import json
import re
import time
from pathlib import Path

from runtime import LABELS, Runtime


def main():
    parser = argparse.ArgumentParser(__doc__)
    parser.add_argument('--model', required=True)
    parser.add_argument('--cases', required=True)
    parser.add_argument('--output', required=True)
    parser.add_argument('--quantize', action='store_true')
    args = parser.parse_args()
    cases = json.loads(Path(args.cases).read_text())
    texts = [' '.join(re.sub('[\u200b\u200c\u200d\ufeff]', '', case['raw']).split())
             for case in cases]
    # Match Go preprocessing and the API's omission of empty working texts.
    texts = [text for text in texts if text]
    texts += ['Астана, улица Туран 55/6', 'John lives in London at 12 Oxford Street.']
    start = time.monotonic()
    runtime = Runtime(args.model, quantize=args.quantize)
    startup = time.monotonic() - start
    import torch
    probes = []
    for labels, threshold in [(list(LABELS.values()), .25),
                              (list(LABELS.values()), .10),
                              (['город', 'улица', 'номер дома', 'квартира', 'подъезд', 'этаж'], .25)]:
        start = time.monotonic()
        try:
            with runtime.lock, torch.inference_mode():
                rows = runtime.model.run(texts, labels, threshold=threshold,
                                         batch_size=runtime.microbatch, flat_ner=False)
        except Exception as error:
            probes.append({'labels': labels, 'threshold': threshold,
                           'seconds': time.monotonic() - start,
                           'error': type(error).__name__ + ': ' + str(error)})
            continue
        probes.append({'labels': labels, 'threshold': threshold,
                       'seconds': time.monotonic() - start,
                       'entities': sum(len(row) for row in rows),
                       'nonempty_rows': sum(bool(row) for row in rows),
                       'rows': rows})
    report = {'quantize': args.quantize, 'startup_seconds': startup,
              'records': len(texts), 'probes': probes,
              'scope': 'Entity yield probe, not accuracy evaluation; last two rows are positive-control texts.'}
    Path(args.output).write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'quantize': args.quantize, 'startup_seconds': startup,
                      'entity_counts': [probe.get('entities', probe.get('error')) for probe in probes]}))


if __name__ == '__main__':
    main()
