ARG PKGNAME

# Build the manager binary
FROM registry.cn-hangzhou.aliyuncs.com/hexiaoshi/golang:1.19.9-alpine as builder

ARG LDFLAGS
ARG PKGNAME

WORKDIR /go/src/github.com/gocrane/crane-scheduler

# Add build deps
RUN apk add build-base

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

ENV GOPROXY="http://goproxy.cn,direct"
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN  go mod download

# Copy the go source
COPY pkg pkg/
COPY cmd cmd/

# Build
RUN go build -ldflags="${LDFLAGS}" -a -o ${PKGNAME} /go/src/github.com/gocrane/crane-scheduler/cmd/${PKGNAME}/main.go

FROM registry.cn-hangzhou.aliyuncs.com/hexiaoshi/alpine:latest
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.ustc.edu.cn/g' /etc/apk/repositories
RUN apk add -U tzdata

WORKDIR /
ARG PKGNAME
COPY --from=builder /go/src/github.com/gocrane/crane-scheduler/${PKGNAME} .
