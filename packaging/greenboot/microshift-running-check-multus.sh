#!/bin/bash
#
# MicroShift Multus-specific functionality used in Greenboot health check procedures.
#
# If 'microshift-multus' RPM is installed, health check needs to include resources
# from the 'openshift-multus' namespace.
#
set -eu -o pipefail

SCRIPT_NAME=$(basename "$0")

# Source the MicroShift health check functions library
# shellcheck source=packaging/greenboot/functions.sh
source /usr/share/microshift/functions/greenboot.sh

# Exit if the current user is not 'root'
if [ "$(id -u)" -ne 0 ] ; then
    echo "The '${SCRIPT_NAME}' script must be run with the 'root' user privileges"
    exit 1
fi

exit_early_if_previous_checks_failed

echo "STARTED"

# Print the boot variable status
print_boot_status

# Set the wait timeout for the current check based on the boot counter
WAIT_TIMEOUT_SECS=$(get_wait_timeout)

if ! microshift healthcheck \
        -v=2 --timeout="${WAIT_TIMEOUT_SECS}s" \
        --namespace openshift-multus \
        --daemonsets multus,dhcp-daemon; then
    create_fail_marker_and_exit
fi
