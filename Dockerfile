FROM golang:1.25 AS builder

ARG VERSION=0.0.0

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/kubeshark/kubeshark/misc.Ver=${VERSION}" -o /out/hub ./cmd/hub

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /out/hub /hub

EXPOSE 8080

ENTRYPOINT ["/hub"]
