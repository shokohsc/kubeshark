FROM golang:1.26 AS builder

ARG VERSION=0.0.0
# The service to build; the Image workflow builds one image per binary
# (hub = backend, front, worker) from this single repo.
ARG BINARY=hub

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/kubeshark/kubeshark/misc.Ver=${VERSION}" -o /out/service ./cmd/${BINARY}

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/service /service

EXPOSE 8080

ENTRYPOINT ["/service"]
