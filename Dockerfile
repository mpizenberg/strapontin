# 1. Frontend
FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build

# 2. Binary
FROM golang:1.27-alpine AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /strapontin .

# 3. Runtime: CA certificates, tzdata, no shell. uid 32767 is the owner Dokku
# gives storage directories by default.
FROM gcr.io/distroless/static-debian12
COPY --from=go /strapontin /strapontin
USER 32767:32767
ENV DATA_DIR=/data PORT=8080
EXPOSE 8080
VOLUME /data
ENTRYPOINT ["/strapontin"]
CMD ["serve"]
