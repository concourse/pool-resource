ARG base_image
ARG builder_image=concourse/golang-builder

FROM ${builder_image} as builder
WORKDIR /src
COPY . .
RUN go mod download
RUN go build -o /assets/out github.com/concourse/pool-resource/cmd/out
RUN set -e; for pkg in $(go list ./...); do \
		go test -o "/tests/$(basename $pkg).test" -c $pkg; \
	done

FROM ${base_image} AS resource
RUN apt update && apt upgrade -y -o Dpkg::Options::="--force-confdef"
RUN apt update && apt install -y jq git make g++ libssl-dev openssh-client
RUN git config --global user.email "git@localhost"
RUN git config --global user.name "git"

ADD assets/ /opt/resource/
RUN chmod +x /opt/resource/*
COPY --from=builder /assets /opt/go
RUN chmod +x /opt/go/out

WORKDIR /root
RUN git clone https://github.com/proxytunnel/proxytunnel.git && \
    cd proxytunnel && \
    make -j4 && \
    install -c proxytunnel /usr/bin/proxytunnel && \
    cd .. && \
    rm -rf proxytunnel

FROM resource AS tests
COPY --from=builder /tests /go/resource-tests/
RUN set -e; for test in /go/resource-tests/*.test; do \
		$test; \
	done
ADD test/ /opt/resource-tests
RUN /opt/resource-tests/all.sh

FROM resource AS integrationtests
RUN apt update && apt install -y squid net-tools
ADD test/ /opt/resource-tests/test
ADD integration-tests /opt/resource-tests/integration-tests
RUN /opt/resource-tests/integration-tests/integration.sh

FROM resource
