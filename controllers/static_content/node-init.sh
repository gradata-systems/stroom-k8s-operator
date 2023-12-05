#!/bin/bash
# Run by the node init container prior to startup.
# Acquires an API token for use by the pod pre-start hook.

source /stroom/scripts/utils.sh

get_auth_token
