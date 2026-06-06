#!/bin/bash
# Executed by the pod pre-start hook.

source /stroom/scripts/utils.sh

log_file='/stroom/logs/k8s/node-start.log'

mkdir -p "$(dirname $log_file)"

# Enable the node
call_api node/v1/enabled/"${STROOM_NODE}" -X PUT -d true
log "Node ${STROOM_NODE} enabled" $log_file

set_job_status "${STROOM_NODE}" 'true'
log "Node ${STROOM_NODE} jobs enabled" $log_file