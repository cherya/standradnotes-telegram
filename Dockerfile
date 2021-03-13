FROM golang:alpine AS builder
WORKDIR /src
ENV CGO_ENABLED=0
COPY . .
RUN go build -o /bin/sntg cmd/standardnotes-telegram/main.go

FROM scratch
COPY --from=builder /bin/sntg /bin/sntg
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY .env .
ENTRYPOINT ["/bin/sntg"]