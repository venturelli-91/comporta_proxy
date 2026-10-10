# Painel: build do React, embutido no binário Go pelo go:embed.
FROM node:24-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Build: binário estático (sem cgo, graças ao modernc.org/sqlite).
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /comporta ./cmd/comporta \
 && mkdir -p /dados && chown 65532:65532 /dados

# Runtime: distroless sem shell, usuário não-root. Imagem pequena e com pouca superfície de ataque.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /comporta /comporta
COPY --from=build --chown=65532:65532 /dados /dados
COPY comporta.json /comporta.json
ENV CONFIG_PATH=/comporta.json DB_PATH=/dados/comporta.db PORT=8080 GOMEMLIMIT=200MiB
EXPOSE 8080
ENTRYPOINT ["/comporta"]
