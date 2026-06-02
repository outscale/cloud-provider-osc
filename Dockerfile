# Copyright 2018 The Kubernetes Authors.

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#     http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

################################################################################
##                               BUILD ARGS                                   ##
################################################################################
# This build arg allows the specification of a custom Golang image.
ARG GOLANG_IMAGE=golang:1.26.3@sha256:2d6c80227255c3112a4d08e67ba98e58efd3846daf15d9d7d4c389565d881b1a

# The distroless image on which the CPI manager image is built.
#
# Please do not use "latest". Explicit tags should be used to provide
# deterministic builds. This image doesn't have semantic version tags, but
# the fully-qualified image can be obtained by entering
# "gcr.io/distroless/static:latest" in a browser and then copying the
# fully-qualified image from the web page.
ARG DISTROLESS_IMAGE=gcr.io/distroless/static@sha256:3592aa8171c77482f62bbc4164e6a2d141c6122554ace66e5cc910cadb961ff0

################################################################################
##                              BUILD STAGE                                   ##
################################################################################
# Build the manager as a statically compiled binary so it has no dependencies
# libc, muscl, etc.
FROM ${GOLANG_IMAGE} AS builder

# This build arg is the version to embed in the CPI binary
ARG VERSION=${VERSION}

# This build arg controls the GOPROXY setting
ARG GOPROXY

WORKDIR /build
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
ENV GOPROXY=${GOPROXY:-https://proxy.golang.org}
RUN make build

################################################################################
##                               MAIN STAGE                                   ##
################################################################################
# Copy the manager into the distroless image.
FROM ${DISTROLESS_IMAGE}
COPY --from=builder /build/osc-cloud-controller-manager /bin/osc-cloud-controller-manager
COPY --from=builder /build/osc-labeler /bin/osc-labeler
ENTRYPOINT [ "/bin/osc-cloud-controller-manager" ]
