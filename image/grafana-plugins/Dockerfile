# syntax=docker/dockerfile:1.7

# Must be declared before the first FROM: an ARG declared between later stages is not
# visible to a subsequent FROM (verified: even its own default value fails to resolve).
ARG GRAFANA_VERSION=13.1.0

FROM node:22-alpine AS datasource-build
WORKDIR /build/unifabric-unifabrictopology-datasource
COPY plugin/unifabric-unifabrictopology-datasource/package.json plugin/unifabric-unifabrictopology-datasource/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci
COPY plugin/unifabric-unifabrictopology-datasource/ ./
RUN npm run build

FROM node:22-alpine AS panel-build
WORKDIR /build/unifabric-unifabrictopology-panel
COPY plugin/unifabric-unifabrictopology-panel/package.json plugin/unifabric-unifabrictopology-panel/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci
COPY plugin/unifabric-unifabrictopology-panel/ ./
RUN npm run build

FROM grafana/grafana:${GRAFANA_VERSION}

# Both plugins are unsigned (no Grafana access policy token configured for signing yet).
ENV GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS="unifabric-unifabrictopology-datasource,unifabric-unifabrictopology-panel"

COPY --chown=grafana:grafana --from=datasource-build /build/unifabric-unifabrictopology-datasource/dist /var/lib/grafana/plugins/unifabric-unifabrictopology-datasource
COPY --chown=grafana:grafana --from=panel-build /build/unifabric-unifabrictopology-panel/dist /var/lib/grafana/plugins/unifabric-unifabrictopology-panel
