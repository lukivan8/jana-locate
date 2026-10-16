FROM python:3.11-slim-bookworm AS build
WORKDIR /app
COPY requirements.lock ./
# Separate CPU wheel index prevents downloading the CUDA runtime on Linux.
RUN pip install --no-cache-dir --extra-index-url https://download.pytorch.org/whl/cpu -r requirements.lock
COPY prepare_model.py ./
RUN HF_HUB_DISABLE_XET=1 python prepare_model.py --output /models/gliner

FROM python:3.11-slim-bookworm
RUN groupadd -g 10001 jana && useradd -u 10001 -g jana -M jana
WORKDIR /app
COPY --from=build /usr/local/lib/python3.11/site-packages /usr/local/lib/python3.11/site-packages
COPY --from=build /usr/local/bin/uvicorn /usr/local/bin/uvicorn
COPY --from=build /models /models
COPY app.py runtime.py ./
USER 10001:10001
ENV HF_HUB_OFFLINE=1 TRANSFORMERS_OFFLINE=1 HF_HUB_DISABLE_TELEMETRY=1 TOKENIZERS_PARALLELISM=false CUDA_VISIBLE_DEVICES="" GLINER_MODEL_PATH=/models/gliner PYTHONDONTWRITEBYTECODE=1
EXPOSE 8090
HEALTHCHECK --interval=5s --timeout=3s --start-period=120s CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8090/readyz',timeout=2)"]
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8090", "--workers", "1", "--no-access-log"]
