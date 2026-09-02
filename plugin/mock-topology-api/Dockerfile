# Build from the repository root context: the mock is part of the main Go
# module so it can reuse pkg/topologyapi.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY pkg/ pkg/
COPY plugin/mock-topology-api/ plugin/mock-topology-api/
RUN CGO_ENABLED=0 go build -o /out/mock-topology-api ./plugin/mock-topology-api

FROM alpine:3.20
COPY --from=build /out/mock-topology-api /usr/local/bin/mock-topology-api
EXPOSE 8080
ENTRYPOINT ["mock-topology-api"]
