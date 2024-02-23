#!/usr/bin/env python3

"""Release Note Tool

This script partially automates the process of publishing release
notes for engineering candidate and release candidate builds of
MicroShift.

The script expects to be run in multiple phases.

*Tagging*

First, it looks for the most recent RPM to have been published to the
mirror, and uses information encoded in that filename to determine the
SHA of the commit that was used for the build and the version number
given to it.

Then it looks for that tag in the local repository. If there is no tag
already, it emits instructions for tagging the correct commit and
pushing the tag to GitHub.

*Draft Release*

After the tag is present, running the script again causes it to use gh
to produce a draft release with a preamble that includes download URLs
and a body that is auto-generated by GitHub's service based on the
pull requests that have merged since the last tagged release.

*Publishing Release*

The script creates a draft release, which must be published by hand to
make it public. Open the link printed at the end of the script run and
use the web interface to review and then publish the release.

NOTE:

  To use this script, you must have the GitHub command line tool "gh"
  installed and you must have enough privileges on the
  openshift/microshift repository to create releases.

"""

import argparse
import collections
import datetime
import html.parser
import os
import re
import subprocess
import textwrap
from urllib import request

URL_BASE = "https://mirror.openshift.com/pub/openshift-v4/aarch64/microshift"
URL_BASE_X86 = "https://mirror.openshift.com/pub/openshift-v4/x86_64/microshift"

# An EC RPM filename looks like
# microshift-4.13.0~ec.4-202303070857.p0.gcf0bce2.assembly.ec.4.el9.aarch64.rpm
# an RC RPM filename looks like
# microshift-4.13.0~rc.0-202303212136.p0.gbd6fb96.assembly.rc.0.el9.aarch64.rpm
VERSION_RE = re.compile(
    r"""
    microshift-                             # prefix
    (?P<full_version>
      (?P<product_version>\d+\.\d+\.\d+)    # product version
      ~                                     # separator
      (?P<candidate_type>ec|rc)\.(?P<candidate_number>\d+)  # which candidate of which type
      -
      (?P<release_date>\d+)\.               # date
      p(?P<patch_num>\d+)\.                 # patch number
      g(?P<commit_sha>[\dabcdef]+)          # commit SHA prefix
    )\.
    """,
    re.VERBOSE,
)

# Include the major.minor version string in this list to ignore
# processing very old versions for which we do not anticipate future
# candidate builds.
OLD_VERSIONS = ['4.12', '4.13', '4.14']


# Representation of one release
Release = collections.namedtuple(
    'Release',
    "release_name commit_sha product_version candidate_type candidate_number release_type release_date",
)


def main():
    """
    The main function of the script. It runs the `check_one()` function for both 'ocp-dev-preview'
    and 'ocp' release types and for a specified version depending upon provided arguments.
    """
    parser = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        '--ec',
        action='store_true',
        default=True,
        dest='ec',
        help='Include engineering candidates (default)',
    )
    parser.add_argument(
        '--no-ec',
        action='store_false',
        dest='ec',
        help='Do not include engineering candidates',
    )
    parser.add_argument(
        '--rc',
        action='store_true',
        default=True,
        help='Include release candidates (default)',
    )
    parser.add_argument(
        '--no-rc',
        action='store_false',
        dest='rc',
        help='Do not include release candidates',
    )
    parser.add_argument(
        '-n', '--dry-run',
        action='store_true',
        dest='dry_run',
        default=False,
        help='Report but take no action',
    )
    args = parser.parse_args()

    new_releases = []
    if args.ec:
        new_releases.extend(find_new_releases(URL_BASE, 'ocp-dev-preview'))
        new_releases.extend(find_new_releases(URL_BASE_X86, 'ocp-dev-preview'))
    if args.rc:
        new_releases.extend(find_new_releases(URL_BASE, 'ocp'))
        new_releases.extend(find_new_releases(URL_BASE_X86, 'ocp'))

    if not new_releases:
        print("No new releases found.")
        return

    print()

    unique_releases = {
        r.commit_sha: r
        for r in new_releases
    }

    for new_release in unique_releases.values():
        publish_release(new_release, not args.dry_run)


class VersionListParser(html.parser.HTMLParser):
    """HTMLParser to extract version numbers from the mirror file list pages.

    A page like https://mirror.openshift.com/pub/openshift-v4/aarch64/microshift/ocp-dev-preview/

    contains HTML like

        <tr class="file">
            <td></td>
            <td>
                <a href="4.12.0-rc.6/">
                    <svg width="1.5em" height="1em" version="1.1" viewBox="0 0 265 323"><use xlink:href="#folder"></use></svg>
                    <span class="name">4.12.0-rc.6</span>
                </a>
            </td>
            <td data-order="-1">&mdash;</td>
            <td class="hideable"><time datetime="">-</time></td>
            <td class="hideable"></td>
        </tr>

    so we look for the 'span' tags with class 'name' and extract the
    text between the tags as the version.
    """

    def __init__(self):
        super().__init__()
        self._in_version = False
        self.versions = []

    def handle_starttag(self, tag, attrs):
        if tag != 'span':
            return
        attr_d = dict(attrs)
        self._in_version = attr_d.get('class', '') == 'name'

    def handle_endtag(self, tag):
        self._in_version = False

    def handle_data(self, data):
        if not self._in_version:
            return
        data = data.strip()
        if not data:
            return
        if data.startswith('latest-'):
            return
        self.versions.append(data)

    def error(self, message):
        "Handle an error processing the HTML"
        print(f"WARNING: error processing HTML: {message}")


def find_new_releases(url_base, release_type):
    """Returns a list of Release instances for missing releases.
    """
    new_releases = []
    # Get the list of the latest RPMs for the release type and vbersion.
    version_list_url = f"{url_base}/{release_type}/"
    with request.urlopen(version_list_url) as response:
        content = response.read().decode("utf-8")
    parser = VersionListParser()
    parser.feed(content)
    for version in parser.versions:
        # Skip very old RCs, indicated by the first 2 parts of the
        # version string major.minor.
        version_prefix = '.'.join(version.split('.')[:2])
        if version_prefix in OLD_VERSIONS:
            continue
        try:
            nr = check_for_new_releases(url_base, release_type, version)
            if nr:
                new_releases.append(nr)
        except Exception as err:  # pylint: disable=broad-except
            print(f"WARNING: could not process {release_type} {version}: {err}")
    return new_releases


def check_for_new_releases(url_base, release_type, version):
    """
    Checks the latest RPMs for a given release type and version,
    and returns a Release instance for any that don't exist.
    """
    # Get the list of the latest RPMs for the release type and
    # version. Different versions use different "os name" components
    # in the path.
    for os_name in ['el9', 'elrhel-9']:
        rpm_list_url = f"{url_base}/{release_type}/{version}/{os_name}/os/rpm_list"
        print(f"\nFetching {rpm_list_url} ...")
        try:
            with request.urlopen(rpm_list_url) as rpm_list_response:
                rpm_list = rpm_list_response.read().decode("utf-8").splitlines()
        except Exception as err:
            print(err)
        else:
            break

    # Look for the RPM for MicroShift itself, with a name like
    #
    # Packages/microshift-4.13.0~ec.3-202302130757.p0.ge636e15.assembly.ec.3.el8__aarch64/microshift-4.13.0~ec.3-202302130757.p0.ge636e15.assembly.ec.3.el8.aarch64.rpm
    #
    # then parse out the EC version number and other details needed to
    # build the release tag.
    version_prefix = version.partition('-')[0]
    microshift_rpm_name_prefix = f"microshift-{version_prefix}"
    microshift_rpm_filename = None
    for package_path in rpm_list:
        parts = package_path.split("/")
        if parts[-1].startswith(microshift_rpm_name_prefix):
            microshift_rpm_filename = parts[-1]
            break
    else:
        rpm_names = ',\n'.join(rpm_list)
        print(f"WARNING: Did not find {microshift_rpm_name_prefix} in {rpm_names}")
        return None

    print(f"Examining RPM {microshift_rpm_filename}")

    match = VERSION_RE.search(microshift_rpm_filename)
    if match is None:
        raise RuntimeError(f"Could not parse version info from '{microshift_rpm_filename}'")
    rpm_version_details = match.groupdict()
    product_version = rpm_version_details["product_version"]
    candidate_type = rpm_version_details["candidate_type"]
    candidate_number = rpm_version_details["candidate_number"]
    release_date = rpm_version_details["release_date"]
    patch_number = rpm_version_details["patch_num"]
    commit_sha = rpm_version_details["commit_sha"]

    # Older release names # look like "4.13.0-ec-2" but we had a few
    # sprints where we published multiple builds, so use more of the
    # version details as the release name now.
    #
    # 4.14.0~ec.3-202307170726.p0
    release_name = f"{product_version}-{candidate_type}.{candidate_number}-{release_date}.p{patch_number}"

    # Check if the release already exists
    print(f"Checking for release {release_name}...")
    try:
        subprocess.run(["gh", "release", "view", release_name],
                       check=True,
                       stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL,
                       )
    except subprocess.CalledProcessError:
        print("Not found")
    else:
        print("Found an existing release, no work to do")
        return None

    return Release(
        release_name,
        commit_sha,
        product_version,
        candidate_type,
        candidate_number,
        release_type,
        release_date,
    )


def tag_exists(release_name):
    "Checks if a given tag exists in the local repository."
    try:
        subprocess.run(["git", "show", release_name],
                       stdout=subprocess.DEVNULL,
                       stderr=subprocess.DEVNULL,
                       check=True)
        return True
    except subprocess.CalledProcessError:
        return False


def tag_release(tag, sha, buildtime):
    env = {}
    # Include our existing environment settings to ensure values like
    # HOME and other git settings are propagated.
    env.update(os.environ)
    timestamp = buildtime.strftime('%Y-%m-%d %H:%M')
    env['GIT_COMMITTER_DATE'] = timestamp
    print(f'GIT_COMMITTER_DATE={timestamp} git tag -s {tag} {sha}')
    subprocess.run(
        ['git', 'tag', '-s', '-m', tag, tag, sha],
        env=env,
        check=True,
    )
    print(f'git push origin {tag}')
    subprocess.run(
        ['git', 'push', 'origin', tag],
        env=env,
        check=True,
    )


def publish_release(new_release, take_action):
    """Does the work to tag and publish a release.
    """
    release_name = new_release.release_name
    commit_sha = new_release.commit_sha
    product_version = new_release.product_version
    candidate_type = new_release.candidate_type
    candidate_number = new_release.candidate_number
    release_type = new_release.release_type
    release_date = new_release.release_date

    if not take_action:
        print('Dry run for new release {new_release} on commit {commit_sha} from {release_date}')
        return

    if not tag_exists(release_name):
        # release_date looks like 202402022103
        buildtime = datetime.datetime.strptime(release_date, '%Y%m%d%H%M')
        tag_release(release_name, commit_sha, buildtime)

    # Set up the release notes preamble with download links
    notes = textwrap.dedent(f"""
    This is a candidate release for {product_version}.

    See the mirror for build artifacts:
    - {URL_BASE_X86}/{release_type}/{product_version}-{candidate_type}.{candidate_number}/
    - {URL_BASE}/{release_type}/{product_version}-{candidate_type}.{candidate_number}/

    """)

    # Create draft release with message that includes download URLs and history
    try:
        subprocess.run(["gh", "release", "create",
                        "--prerelease",
                        "--notes", notes,
                        "--generate-notes",
                        release_name,
                        ],
                       check=True)
    except subprocess.CalledProcessError as err:
        print(f"Failed to create the release: {err}")


if __name__ == "__main__":
    main()
