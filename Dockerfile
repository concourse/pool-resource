ARG base_image=cgr.dev/chainguard/wolfi-base
ARG builder_image=concourse/golang-builder

ARG BUILDPLATFORM
FROM --platform=${BUILDPLATFORM} ${builder_image} AS builder

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=$TARGETOS
ENV GOARCH=$TARGETARCH

WORKDIR /src
COPY . .
RUN go mod download
RUN go build -o /assets/out ./cmd/out
RUN set -e; for pkg in $(go list ./...); do \
		go test -o "/tests/$(basename $pkg).test" -c $pkg; \
	done

FROM ${base_image} AS proxybuilder
RUN apk --no-cache add \
    git \
    gcc \
    make \
    openssl-dev

WORKDIR /root
RUN git clone https://github.com/proxytunnel/proxytunnel.git && \
    cd proxytunnel && \
    make -j4 && \
    install -c proxytunnel /usr/bin/proxytunnel && \
    cd .. && \
    rm -rf proxytunnel

FROM ${base_image} AS resource
RUN apk --no-cache add \
    bash \
    jq \
    git \
    openssh-client
RUN git config --global user.email "git@localhost"
RUN git config --global user.name "git"

ADD assets/ /opt/resource/
RUN chmod +x /opt/resource/*
COPY --from=builder /assets /opt/go
RUN chmod +x /opt/go/out

COPY --from=proxybuilder /usr/bin/proxytunnel /usr/bin/

FROM resource AS tests
RUN apk --no-cache add git-daemon cmd:ssh-keygen
COPY --from=builder /tests /go/resource-tests/
RUN set -e; for test in /go/resource-tests/*.test; do \
		$test; \
	done
ADD test/ /opt/resource-tests
RUN /opt/resource-tests/all.sh

FROM resource AS integrationtests
RUN apk --no-cache add squid net-tools
ADD test/ /opt/resource-tests/test
ADD integration-tests /opt/resource-tests/integration-tests
RUN /opt/resource-tests/integration-tests/integration.sh

FROM resource
