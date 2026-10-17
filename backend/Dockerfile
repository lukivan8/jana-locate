FROM debian:bookworm-slim AS postal
ARG LIBPOSTAL_COMMIT=25099c506612b34b23b1bfe286ca6321fcf06f35
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl git build-essential autoconf automake libtool pkg-config && rm -rf /var/lib/apt/lists/*
RUN git clone https://github.com/openvenues/libpostal /src && cd /src && git checkout "$LIBPOSTAL_COMMIT"
WORKDIR /src
RUN ./bootstrap.sh && ./configure --prefix=/opt/libpostal --datadir=/models --disable-sse2 --disable-data-download && make -j2 && make install
# All model assets are prepared at build time; startup never downloads data.
RUN ./src/libpostal_data download base /models/libpostal && ./src/libpostal_data download parser /models/libpostal

FROM golang:1.23-bookworm AS build
COPY --from=postal /opt/libpostal /opt/libpostal
ENV CGO_ENABLED=1 CGO_CFLAGS=-I/opt/libpostal/include CGO_LDFLAGS="-L/opt/libpostal/lib -Wl,-rpath,/opt/libpostal/lib"
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test -tags libpostal ./... && go build -tags libpostal -trimpath -ldflags="-s -w" -o /out/jana-address ./cmd/server

FROM python:3.11-slim-bookworm AS reference
WORKDIR /prepare
COPY data/reference /input
COPY tools/build_reference.py tools/district_context.py ./
RUN python build_reference.py --input /input/entities.jsonl --manifest /input/manifest.json --output /out/reference.sqlite

FROM debian:bookworm-slim
RUN groupadd -g 10001 jana && useradd -u 10001 -g jana -M jana && mkdir -p /data /app/data && chown jana:jana /data
WORKDIR /app
COPY --from=postal /opt/libpostal/lib/libpostal.so* /opt/libpostal/lib/
COPY --from=postal /models/libpostal /models/libpostal
COPY --from=build /out/jana-address /app/jana-address
COPY --from=reference /out/reference.sqlite /app/data/reference.sqlite
COPY data/catalog.seed.json /app/data/catalog.seed.json
USER 10001:10001
ENV HTTP_ADDR=:8080 CATALOG_SEED_PATH=/app/data/catalog.seed.json CATALOG_OVERLAY_PATH=/data/catalog.confirmed.json LIBPOSTAL_DATA_DIR=/models/libpostal REFERENCE_PATH=/app/data/reference.sqlite MODELS_REQUIRED=true ALLOW_DEMO_CATALOG=false LD_LIBRARY_PATH=/opt/libpostal/lib
ARG JANA_REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/lukivan8/jana-locate" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.revision="$JANA_REVISION"
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s --start-period=30s CMD ["/app/jana-address", "healthcheck"]
ENTRYPOINT ["/app/jana-address"]
