FROM golang:1.20 AS builder_source
ARG GITHUB_USER=Kingdo777
ARG DATA_FUNCTION_MANAGER_GITHUB_BRANCH=master
RUN git clone --branch ${DATA_FUNCTION_MANAGER_GITHUB_BRANCH} \
    https://github.com/${GITHUB_USER}/data-function-manager.git /src/data-function-manager;\
    cd /src/data-function-manager; env GO111MODULE=on CGO_ENABLED=0 go build main/proxy.go &&\
    mv proxy /bin/proxy_source

COPY . /src/local/data-function-manager
RUN cd /src/local/data-function-manager; env GO111MODULE=on CGO_ENABLED=0 go build main/proxy.go &&\
    mv proxy /bin/proxy_source_local

ARG GITHUB_USER=Kingdo777
ARG DATA_FUNCTION_GITHUB_BRANCH=master
RUN git clone --branch ${DATA_FUNCTION_GITHUB_BRANCH} \
    https://github.com/${GITHUB_USER}/data-function.git /src/data-function

FROM ubuntu:22.04

# select the builder to use
ARG GO_PROXY_BUILD_FROM=proxy_source_local

COPY --from=builder_source /bin/proxy_source /bin/proxy_source
COPY --from=builder_source /bin/proxy_source_local /bin/proxy_source_local
RUN mv /bin/${GO_PROXY_BUILD_FROM} /bin/proxy

COPY --from=builder_source /src/data-function/src/DataFunction/action/__main__.py /action/__main__.py
ENV DataFunctionActionCodePath="/action/__main__.py"

ENV Openwhisk_ApiHost="222.20.94.67"
ENV Openwhisk_AuthName="23bc46b1-71f6-4ed5-8c54-816aa4f8c502"
ENV Openwhisk_AuthPassword="123zO3xZCLrMN6v2BKK1dXYFpXlPkccOFqm12CdAsMgRU4VrNZ9lyGVCGuMDGIwP"

ENTRYPOINT ["/bin/proxy", "-debug"]