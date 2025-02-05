#!/usr/bin/env bash

set -o errexit
set -o errtrace
set -o nounset
set -o pipefail

shopt -s expand_aliases
shopt -s extglob

export PS4='+ $(date "+%T.%N") ${BASH_SOURCE#$HOME/}:$LINENO \011'

REPOROOT="$(readlink -f "$(dirname "${BASH_SOURCE[0]}")/../..")"
STAGING_ROOT="${REPOROOT}/_output/staging"
STAGING_RHOAI="${STAGING_ROOT}/rhoai"
STAGING_BUNDLE="${STAGING_RHOAI}/bundle"
STAGING_OPERATOR="${STAGING_RHOAI}/operator"

CSV_FILENAME="rhods-operator.clusterserviceversion.yaml"
PULL_SECRET_FILE="${HOME}/.pull-secret.json"

RELEASE_JSON="${REPOROOT}/assets/optional/rhoai/release-x86_64.json"

title() {
    echo -e "\E[34m$1\E[00m";
}

check_preconditions() {
    if ! hash yq; then
        title "Installing yq"

        local YQ_VER=4.26.1
        # shellcheck disable=SC2034  # appears unused
        local YQ_HASH_amd64=9e35b817e7cdc358c1fcd8498f3872db169c3303b61645cc1faf972990f37582
        # shellcheck disable=SC2034  # appears unused
        local YQ_HASH_arm64=8966f9698a9bc321eae6745ffc5129b5e1b509017d3f710ee0eccec4f5568766
        local YQ_HASH
        YQ_HASH="YQ_HASH_$(go env GOARCH)"
        local YQ_URL
        YQ_URL="https://github.com/mikefarah/yq/releases/download/v${YQ_VER}/yq_linux_$(go env GOARCH)"
        local YQ_EXE
        YQ_EXE=$(mktemp /tmp/yq-exe.XXXXX)
        local YQ_SUM
        YQ_SUM=$(mktemp /tmp/yq-sum.XXXXX)
        echo -n "${!YQ_HASH} -" > "${YQ_SUM}"
        if ! (curl -Ls "${YQ_URL}" | tee "${YQ_EXE}" | sha256sum -c "${YQ_SUM}" &>/dev/null); then
            echo "ERROR: Expected file at ${YQ_URL} to have checksum ${!YQ_HASH} but instead got $(sha256sum <"${YQ_EXE}" | cut -d' ' -f1)"
            exit 1
        fi
        chmod +x "${YQ_EXE}" && sudo cp "${YQ_EXE}" /usr/bin/yq
        rm -f "${YQ_EXE}" "${YQ_SUM}"
    fi

    if ! hash python3; then
        echo "ERROR: python3 is not present on the system - please install"
        exit 1
    fi

    if ! python3 -c "import yaml"; then
        echo "ERROR: missing python's yaml library - please install"
        exit 1
    fi

    if ! hash oc; then
        echo "ERROR: oc is not present on the system - please install"
        exit 1
    fi
}

get_auth() {
    if [ -f "${PULL_SECRET_FILE}" ]; then
        echo "--registry-config ${PULL_SECRET_FILE}"
    else
        >&2 echo "Warning: no pull secret found at ${PULL_SECRET_FILE}"
        echo ""
    fi
}

# download_rhoai_manifests() fetches the RHOAI's kserve and runtime manifests.
# First, it downloads the RHOAI Operator bundle CSV and extracts image ref to the RHOAI Operator image.
# Then, it extracts the manifests from the RHOAI Operator image to the staging dir.
# No processing is done in this functions.
download_rhoai_manifests() {
    local -r bundle_ref="${1}"

    # Jan 2025: Only x86_64 is supported (https://access.redhat.com/articles/rhoai-supported-configs)
    # therefore there's no loop over architectures.

    rm -rf "${STAGING_RHOAI}" && mkdir -p "${STAGING_BUNDLE}" "${STAGING_OPERATOR}"
    local -r authentication="$(get_auth)"

    title "Fetching RHOAI CSV"
    oc image extract \
        ${authentication} \
        --path "/manifests/${CSV_FILENAME}:${STAGING_BUNDLE}" \
        "${bundle_ref}" \
        --filter-by-os amd64 || return 1

    local -r operator_ref=$(yq '.spec.relatedImages[] | select(.name == "odh-rhel8-operator-*") | .image' "${STAGING_BUNDLE}/${CSV_FILENAME}")
    title "Fetching RHOAI manifests"
    oc image extract \
        ${authentication} \
        --path "/opt/manifests/:${STAGING_OPERATOR}" \
        "${operator_ref}" \
        --filter-by-os amd64 || return 1
}

process_rhoai_manifests() {
    title "Copying manifests from staging dir to assets/"
    "${REPOROOT}/scripts/auto-rebase/handle_assets.py" ./scripts/auto-rebase/assets_rhoai.yaml

    title "Initializing release.json file"
    local -r version=$(yq '.spec.version' "${STAGING_BUNDLE}/${CSV_FILENAME}")
    echo "{ \"release\": {\"base\": \"${version}\"}, \"images\": {}}" | yq -o json > "${RELEASE_JSON}"

    update_runtimes
    update_kserve
}

update_kserve() {
    local -r kserve_images=$(cat ${STAGING_OPERATOR}/kserve/overlays/odh/params.env)
    for image in ${kserve_images}; do
        local image_name="${image%=*}"
        local image_ref="${image#*=}"
        yq -i ".images.${image_name} = \"${image_ref}\"" "${RELEASE_JSON}"
    done
}

update_runtimes() {
    title "Dropping template containers from ServingRuntimes and changing them to ClusterServingRuntimes"
    for runtime in $(find "${REPOROOT}/assets/optional/rhoai/runtimes/" -iname '*.yaml' -not -name 'kustomization.yaml'); do
        yq --inplace '.objects[0] | .kind = "ClusterServingRuntime"' "${runtime}"
        containers_amount=$(yq '.spec.containers | length' "${runtime}")
        for ((i=0; i<containers_amount; i++)); do
            idx="${i}" yq --inplace --string-interpolation=false \
                '.spec.containers[env(idx)].image |= sub("\$\((.*)\)", "${1}")' \
                "${runtime}"
        done
    done

    title "Creating ServingRuntime images kustomization"

    local -r kustomization_images="${REPOROOT}/assets/optional/rhoai/runtimes/kustomization.x86_64.yaml"
    cat <<EOF > "${kustomization_images}"

images:
EOF

    local -r images=$(cat "${STAGING_OPERATOR}"/odh-model-controller/base/*.env | grep "\-image")
    for image in ${images}; do
        local image_name="${image%=*}"
        local image_ref="${image#*=}"
        local image_ref_repo="${image_ref%@*}"
        local image_ref_digest="${image_ref#*@}"

        tee -a "${kustomization_images}" <<EOF
  - name: ${image_name}
    newName: ${image_ref_repo}
    digest: ${image_ref_digest}
EOF

        yq -i ".images.${image_name} = \"${image_ref}\"" "${RELEASE_JSON}"
    done
}

download_rhoai_manifests "registry.redhat.io/rhoai/odh-operator-bundle:v2.16.0"
process_rhoai_manifests
