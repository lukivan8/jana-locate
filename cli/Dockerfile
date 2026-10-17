FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/jana ./cmd/jana && mkdir -p /out/data && chown 10001:10001 /out/data

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/jana /jana
COPY --from=build --chown=10001:10001 /out/data /data
USER 10001:10001
ENV JANA_STATE_PATH=/data/workspace.sqlite
EXPOSE 8095
HEALTHCHECK --interval=5s --timeout=3s CMD ["/jana", "healthcheck"]
ENTRYPOINT ["/jana"]
CMD ["serve", "--listen", "0.0.0.0:8095"]
