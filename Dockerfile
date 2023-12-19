FROM golang:1.20 AS builder_source
ARG GITHUB_USER=Kingdo777
ARG STATE_FUNCTION_MANAGER_GITHUB_BRANCH=master
RUN git clone --branch ${STATE_FUNCTION_MANAGER_GITHUB_BRANCH} \
    https://github.com/${GITHUB_USER}/state-function-manager.git /src/state-function-manager;\
    cd /src/state-function-manager; env GO111MODULE=on CGO_ENABLED=0 go build main/proxy.go &&\
    mv proxy /bin/proxy_source

COPY . /src/local/state-function-manager
RUN cd /src/local/state-function-manager; env GO111MODULE=on CGO_ENABLED=0 go build main/proxy.go &&\
    mv proxy /bin/proxy_source_local

ARG GITHUB_USER=Kingdo777
ARG STATE_FUNCTION_GITHUB_BRANCH=master
RUN git clone --branch ${STATE_FUNCTION_GITHUB_BRANCH} \
    https://github.com/${GITHUB_USER}/state-function.git /src/state-function

FROM ubuntu:22.04

# select the builder to use
ARG GO_PROXY_BUILD_FROM=proxy_source_local

COPY --from=builder_source /bin/proxy_source /bin/proxy_source
COPY --from=builder_source /bin/proxy_source_local /bin/proxy_source_local
RUN mv /bin/${GO_PROXY_BUILD_FROM} /bin/proxy

COPY --from=builder_source /src/state-function/src/StateFunction/action/__main__.py /action/__main__.py
ENV StateFunctionActionCodePath="/action/__main__.py"

ENV Openwhisk_ApiHost="222.20.94.67"
ENV Openwhisk_AuthName="23bc46b1-71f6-4ed5-8c54-816aa4f8c502"
ENV Openwhisk_AuthPassword="123zO3xZCLrMN6v2BKK1dXYFpXlPkccOFqm12CdAsMgRU4VrNZ9lyGVCGuMDGIwP"

ENTRYPOINT ["/bin/proxy", "-debug"]