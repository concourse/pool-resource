ARG base_image=alpine:latest
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
RUN apk add --no-cache bash jq git git-daemon openssh
RUN git config --global user.email "git@localhost"
RUN git config --global user.name "git"
ADD assets/ /opt/resource/
RUN chmod +x /opt/resource/*
COPY --from=builder /assets /opt/go
RUN chmod +x /opt/go/out

FROM resource AS tests
COPY --from=builder /tests /go/resource-tests/
RUN set -e; for test in /go/resource-tests/*.test; do \
		$test -ginkgo.v; \
	done
ADD test/ /opt/resource-tests
RUN /opt/resource-tests/all.sh

FROM resource
