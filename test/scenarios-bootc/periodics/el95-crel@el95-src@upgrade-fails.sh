#!/bin/bash

# Sourced from scenario.sh and uses functions defined there.

# Disable signature verification because the test performs an upgrade to
# a target reference unsigned image that was generated by local builds
# shellcheck disable=SC2034  # used elsewhere
IMAGE_SIGSTORE_ENABLED=false

start_image=rhel95-bootc-crel

scenario_create_vms() {
    if ! does_image_exist "${start_image}"; then
        echo "Image '${start_image}' not found - skipping test"
        return 0
    fi
    prepare_kickstart host1 kickstart-bootc.ks.template "${start_image}"
    launch_vm --boot_blueprint rhel95-bootc
}

scenario_remove_vms() {
    if ! does_image_exist "${start_image}"; then
        echo "Image '${start_image}' not found - skipping test"
        return 0
    fi
    remove_vm host1
}

scenario_run_tests() {
    if ! does_image_exist "${start_image}"; then
        echo "Image '${start_image}' not found - skipping test"
        return 0
    fi
    run_tests host1 \
        --variable "FAILING_REF:rhel95-bootc-source" \
        --variable "REASON:fail_greenboot" \
        --variable "BOOTC_REGISTRY:${MIRROR_REGISTRY_URL}" \
        suites/upgrade/upgrade-fails-and-rolls-back.robot
}
