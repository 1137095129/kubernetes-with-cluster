FROM golang AS builder
RUN apt-get update && \
    apt-get install -y --no-install-recommends build-essential && \
    apt-get clean && \
    mkdir -p "$GOPATH/github.com/wang1137095129/kubernetes-with-cluster"
ADD . "$GOPATH/github.com/wang1137095129/kubernetes-with-cluster"
RUN cd "$GOPATH/github.com/wang1137095129/kubernetes-with-cluster" && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a --installsuffix cgo --ldflags="-s" -o /kubernetes-with-cluster

FROM bitnami/minideb:stretch
RUN install_packages ca-certificates
COPY --from=builder /kubernetes-with-cluster /kubernetes-with-cluster
ENTRYPOINT ["/kubernetes-with-cluster","--namespace=default"]