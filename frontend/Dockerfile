FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY . .
RUN npm run build && npm run typecheck

FROM nginxinc/nginx-unprivileged:1.28-alpine
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist/client /usr/share/nginx/html
COPY --chmod=755 docker-entrypoint.sh /docker-entrypoint.d/40-jana-config.sh
# The entrypoint writes only this runtime URL file before serving static assets.
USER root
RUN chown nginx:nginx /usr/share/nginx/html/config.js
USER nginx
ARG JANA_REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/lukivan8/jana-locate" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.revision="$JANA_REVISION"
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
