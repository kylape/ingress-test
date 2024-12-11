FROM registry.redhat.io/rhel9/go-toolset:latest as builder

COPY . .
RUN go build -o ingress-test .

FROM registry.redhat.io/ubi9/ubi:latest

COPY --from=builder /opt/app-root/src/ingress-test /bin/ingress-test

ENTRYPOINT /bin/ingress-test
