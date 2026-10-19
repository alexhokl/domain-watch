FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /domain-watch .

# Minimal runtime image: just the binary and CA certs for TLS to the RDAP
# servers and Cloud Storage.
FROM scratch
COPY --from=build /domain-watch /domain-watch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER 65532:65532

ENTRYPOINT ["/domain-watch"]
