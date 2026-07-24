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

ARG DISTROLESS_IMAGE=gcr.io/distroless/static@sha256:3592aa8171c77482f62bbc4164e6a2d141c6122554ace66e5cc910cadb961ff0

FROM ${DISTROLESS_IMAGE}
ARG TARGETPLATFORM

COPY $TARGETPLATFORM/osc-cloud-controller-manager /bin/osc-cloud-controller-manager
COPY $TARGETPLATFORM/osc-labeler /bin/osc-labeler
ENTRYPOINT [ "/bin/osc-cloud-controller-manager" ]
