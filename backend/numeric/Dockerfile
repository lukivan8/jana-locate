FROM python:3.11-slim-bookworm
# Preparation happens during build. Startup never installs or downloads assets.
RUN pip install --no-cache-dir python-crfsuite==0.9.12
WORKDIR /app
COPY model.crfsuite /app/model.crfsuite
RUN echo '1cf4bdc47fbac61c18ebcedc78bd19022eacaff799a38d1bdc75b886c29dabd4  /app/model.crfsuite' | sha256sum -c -
COPY tokens.py numeric_recovery.py server_numeric.py test_numeric_recovery.py ./
RUN python test_numeric_recovery.py
ENV PYTHONDONTWRITEBYTECODE=1
USER 10001:10001
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=2s CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://localhost:8080/ready',timeout=1).read()"]
ENTRYPOINT ["python", "-u", "/app/server_numeric.py"]
