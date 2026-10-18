# Compact numeric supplement

Selected candidate: the existing native Go pipeline plus an optional numeric-only
CRF service. No XLM-R, second libpostal, reference DB or full-address cache exists in
this sidecar. `server_numeric.py` returns source-grounded spans and token posteriors;
the Go adapter decides whether a null field can be filled. Existing values,
geographic candidates, warnings, status and native confidence remain unchanged.
Explicit ambiguous/conflicting records are left alone. Service failure returns the
native result unchanged. A token posterior is not an address confidence.

Features use Unicode tokens, local lexical context and word/number adjacency;
decimal values are masked. The fixed model was trained on 3540 previously admitted
rows plus 880 synthetic rows. It requires python-crfsuite 0.9.12. Prepared model:
`assets/numeric-recovery/model.crfsuite`, SHA256
`1cf4bdc47fbac61c18ebcedc78bd19022eacaff799a38d1bdc75b886c29dabd4`.
Model hash is checked before startup. Nothing is downloaded at runtime.

Build with `Dockerfile.numeric` using a preprepared CRF dependency image; its package
metadata/license are copied. Set `NUMERIC_SUPPLEMENT_URL` on the native backend.
The root `compose.numeric.yaml` is an explicit opt-in configuration for isolated
verification, not used by the public showcase. It limits the sidecar to 128 MiB and
places it on an internal network. Do not combine with `compose.contextual.yaml`.

Run `python3 backend/contextual/runtime/numeric-recovery/test_numeric_recovery.py`.
Backend tests additionally check preservation, source offsets, int64 IDs, outages,
ambiguity and protection of landmark numbers. Quality, timing and limitations are
recorded in the root `docs/numeric-recovery-results.md`. The ignored laboratory
`.artifacts/normalization-lab/goal/numeric-recovery-v4/` retains the training manifest,
model, raw results, negative experiments, Docker recipes and checksums. Earlier
v7-repair and transformer-agreement alternatives are retained only as experiments;
they are not required by this service. No experimental pipeline is enabled publicly.
