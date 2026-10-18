# Prepared once with Dockerfile.base; contains only pinned dependencies/native assets.
ARG BASE_IMAGE=jana-contextual-base:v7
FROM ${BASE_IMAGE} AS dependencies
FROM python:3.11-slim-bookworm
COPY --from=dependencies /usr/local/lib/python3.11/site-packages /usr/local/lib/python3.11/site-packages
COPY --from=dependencies /opt/libpostal/lib /opt/libpostal/lib
COPY --from=dependencies /models/libpostal /models/libpostal
COPY --from=dependencies /lab-trace /lab-trace
ENV LD_LIBRARY_PATH=/opt/libpostal/lib HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 TOKENIZERS_PARALLELISM=false PYTHONDONTWRITEBYTECODE=1 OMP_NUM_THREADS=2
WORKDIR /lab/goal
COPY assets/ ./
COPY assets-manifest.json /app/assets-manifest.json
RUN python -c 'import hashlib,json,pathlib; root=pathlib.Path("/lab/goal"); manifest=json.load(open("/app/assets-manifest.json")); assert all(hashlib.sha256((root/p).read_bytes()).hexdigest()==h for p,h in manifest.items()), "Asset checksum mismatch"'
COPY runtime/ ./
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --start-period=90s CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://localhost:8080/ready',timeout=2).read()"]
ENTRYPOINT ["python", "-u", "/lab/goal/server.py"]
