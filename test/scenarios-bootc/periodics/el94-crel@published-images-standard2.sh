#!/bin/bash

# Sourced from scenario.sh and uses functions defined there.

scenario_create_vms() {
    if [[ "${CURRENT_RELEASE_REPO}" == http* ]] ; then
        # Discover a pre-release MicroShift bootc image reference on the mirror
        local -r mirror_url="$(dirname "${CURRENT_RELEASE_REPO}")/bootc-pullspec.txt"
        local -r bootc_spec="$(curl -s "${mirror_url}")"

        if [ -z "${bootc_spec}" ] || [[ "${bootc_spec}" != quay.io/openshift* ]] ; then
            echo "ERROR: Failed to retrieve a bootc pull spec from '${mirror_url}'"
            exit 1
        fi
    else
        # Discover a released MicroShift bootc image reference in public registry.
        #
        # TODO: When the current release is updated to 'rhocp', this test will
        # generate an error and will need to be updated to discover the public
        # MicroShift bootc image references.
        echo "ERROR: Code for detecting the released bootc images is missing"
        exit 1
    fi

    prepare_kickstart host1 kickstart-bootc.ks.template "${bootc_spec}"
    launch_vm --boot_blueprint rhel94-bootc --bootc

    # Open the firewall ports. Other scenarios get this behavior by embedding
    # settings in the blueprint, but we cannot open firewall ports in published
    # images. We need to do this step before running the RF suite so that suite
    # can assume it can reach all of the same ports as for any other test.
    configure_vm_firewall host1
}

scenario_remove_vms() {
    remove_vm host1
}

scenario_run_tests() {
    run_tests host1 suites/standard2/
}
