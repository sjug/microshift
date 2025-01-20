#!/bin/bash

# Sourced from scenario.sh and uses functions defined there.

# Disable signature verification because the test performs an upgrade to
# a target reference unsigned image that was generated by local builds
# shellcheck disable=SC2034  # used elsewhere
IMAGE_SIGSTORE_ENABLED=false

scenario_create_vms() {
    # The y-1 ostree image will be fetched from the cache as it is not built
    # as part of the bootc image build procedure
    prepare_kickstart host1 kickstart.ks.template "rhel-9.4-microshift-4.${PREVIOUS_MINOR_VERSION}"
    launch_vm 
}

scenario_remove_vms() {
    remove_vm host1
}

scenario_run_tests() {
    run_tests host1 \
        --variable "TARGET_REF:rhel95-bootc-source" \
        --variable "BOOTC_REGISTRY:${MIRROR_REGISTRY_URL}" \
        suites/upgrade/upgrade-successful.robot
}
