#!/usr/bin/env bash

# Copyright 2020 Arm Limited.
# SPDX-License-Identifier: Apache-2.0
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

# This script sets up the source tree to use Bob Blueprint under Android.

# Bootstrap Bob with a BUILDDIR in the Android output directory, and
# generate an initial config based on the args passed to this script.
# Finally run Bob to generate the Android.bp for the configuration.

SCRIPT_DIR="$(dirname "${BASH_SOURCE[0]}")"
BOB_DIR=bob-build
PROJ_NAME="bob_example"

BASENAME=$(basename $0)
function usage {
    cat <<EOF
$BASENAME

Sets up the Bob to build for Android using Android.bp files.

Usage:
 $BASENAME CONFIG_OPTIONS...
 $BASENAME --menuconfig

  CONFIG_OPTIONS is a list of configuration items that can include .config
  profiles and explicit options (like DEBUG=y)

Options
  -m, --menuconfig  Open configuration editor
  -h, --help        Help text

Examples:
  $BASENAME ANDROID_N=y DEBUG=n
  $BASENAME --menuconfig
EOF
}

MENU=0
PARAMS=$(getopt -o hm -l help,menuconfig --name $(basename "$0") -- "$@")

eval set -- "$PARAMS"
unset PARAMS

while true ; do
    case $1 in
        -m | --menuconfig)
            MENU=1
            shift
            ;;
        --)
            shift
            break
            ;;
        -h | --help | *)
            usage
            exit 1
            ;;
    esac
done

[[ -n ${OUT} ]] || { echo "\$OUT is not set - did you run 'lunch'?"; exit 1; }
[[ -n ${ANDROID_BUILD_TOP} ]] || { echo "\$ANDROID_BUILD_TOP is not set - did you run 'lunch'?"; exit 1; }

source "${SCRIPT_DIR}/${BOB_DIR}/pathtools.bash"

PROJ_DIR=$(relative_path "${ANDROID_BUILD_TOP}" "${SCRIPT_DIR}")

# Change to the working directory
cd "${ANDROID_BUILD_TOP}"

### Variables required for Bob and Android.mk bootstrap ###
BPBUILD_DIR="${OUT}/gen/STATIC_LIBRARIES/bobbp_${PROJ_NAME}_intermediates"
export BUILDDIR="${BPBUILD_DIR}"
export CONFIGNAME="bob.config"
export SRCDIR="${PROJ_DIR}"
export BLUEPRINT_LIST_FILE="${SRCDIR}/bplist"

# Bootstrap Bob
"${PROJ_DIR}/${BOB_DIR}/bootstrap_androidbp.bash"


# Pick up some info that bob has worked out
source "${BUILDDIR}/.bob.bootstrap"

if [ $MENU -ne 1 ] || [ ! -f "${BPBUILD_DIR}/${CONFIGNAME}" ] ; then
    # Have arguments or missing bob.config. Run config.
    "${BPBUILD_DIR}/config" ANDROID=y BUILDER_ANDROID_BP=y "$@"
fi

if [ $MENU -eq 1 ] ; then
    "${BPBUILD_DIR}/menuconfig"
fi

# Once configured, generate the Android.bp by running Bob
# There is a symlink in the build directory.
"${BPBUILD_DIR}/bob"
