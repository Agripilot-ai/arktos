#!/usr/bin/env bash

# Copyright 2014 The Kubernetes Authors.
# Copyright 2020 Authors of Arktos - file modified.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Tear down a Kubernetes cluster.

set -o errexit
set -o nounset
set -o pipefail

KUBE_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

if [ -f "${KUBE_ROOT}/cluster/env.sh" ]; then
    source "${KUBE_ROOT}/cluster/env.sh"
fi

source "${KUBE_ROOT}/cluster/kube-util.sh"

export RESOURCE_DIRECTORY=${RESOURCE_DIRECTORY:-"${KUBE_ROOT}/cluster"}
export SHARED_CA_DIRECTORY=${SHARED_CA_DIRECTORY:-"/tmp/shared_ca"}

echo "Bringing down cluster using provider: $KUBERNETES_PROVIDER"

echo "... calling verify-prereqs" >&2
verify-prereqs
echo "... calling verify-kube-binaries" >&2
verify-kube-binaries
echo "... calling kube-down" >&2

if [[ "${SCALEOUT_CLUSTER:-false}" == "true" ]]; then
    export SCALEOUT_PROXY_NAME="${KUBE_GCE_INSTANCE_PREFIX}-proxy"
    delete-proxy
fi

if [[ ${PRESET_INSTANCES_ENABLED:-false} == $TRUE ]]; then
    kube-down-for-preset-machines
else
    kube-down
fi

echo "Done"
exit 0
