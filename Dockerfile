# syntax=docker/dockerfile:1

FROM golang:1.27-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/server ./cmd/server

# distroless/static instead of scratch: it already carries the CA
# certificates needed for the HTTPS call to openrouter.ai. Everything else
# (markdown, images, templates, CSS/JS, fonts) is already inside the binary
# via go:embed, so nothing else needs to be copied in.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/server /server
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/server"]
