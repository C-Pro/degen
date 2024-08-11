FROM golang as builder

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /go/bin/degen .

FROM alpine

COPY --chown=65534:65534 --from=builder /go/bin/degen /
USER 65534
EXPOSE 8080

ENTRYPOINT [ "/degen" ]
