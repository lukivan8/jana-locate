# Internal GLiNER CPU service

Go is the only application client. Compose publishes no host port for this service; it does not change the public Jana Address OpenAPI contract.

`POST /v1/entities` accepts `{texts: [...]}` and returns `{results: [[{start,end,text,label,score}, ...], ...]}` in input order. Offsets are Unicode codepoints into each working text. Go validates spans before admitting them as evidence. This internal endpoint supplies entities only, never building coordinates or confirmed answers.

One worker loads the multilingual GLiNER model from prepared local files, checks hashes, runs on CPU, and serializes inference. Default: two Torch threads, microbatches of eight, int8 linear layers. Long inputs use overlapping windows and deduplicated spans; the tail is not silently discarded. Readiness is exposed only after loading and warmup. Bodies and text sizes are bounded. Application docs/OpenAPI are disabled because this is an internal transport.

The Docker build downloads pinned weights/tokenizer once; runtime has strict HuggingFace offline settings and a read-only filesystem. No CUDA runtime, network geocoder or model download is needed. See backend normalization documentation for setup, limitations and resource checks.

Preparation uses `prepare_model.py`, runtime uses `runtime.py`, HTTP transport uses `app.py`. `requirements.lock` pins dependencies; Linux uses the explicit CPU PyTorch wheel. To regenerate the lock, preserve that platform-specific CPU requirement rather than accidentally selecting CUDA wheels.

Unit transport/window checks use a stub and are explicitly separate from `verify_offline.py`, which loads real weights with socket connections forbidden and an empty HuggingFace cache. Neither test claims address accuracy.
