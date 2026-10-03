FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM arigaio/atlas:1.3.3-community-alpine

RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app
COPY --from=build --chown=app:app /out/api /app/api
COPY --chown=app:app atlas.hcl /app/atlas.hcl
COPY --chown=app:app migrations/ /app/migrations/
COPY --chown=app:app docker/entrypoint.sh /app/entrypoint.sh

ENV GIN_MODE=release \
    PORT=8080

EXPOSE 8080
USER app
ENTRYPOINT ["/bin/sh", "/app/entrypoint.sh"]
