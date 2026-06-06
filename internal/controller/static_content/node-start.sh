#!/bin/bash
# Executed by the pod pre-start hook.

source /stroom/scripts/utils.sh

log_file='/stroom/logs/k8s/node-start.log'

mkdir -p "$(dirname $log_file)"

# Enable the node
call_api node/v1/enabled/"${STROOM_NODE}" -X PUT -d true
log "Node ${STROOM_NODE} enabled" $log_file

# Enable all node tasks (except if this is a dedicated UI nodes)
enabled='true'
if [[ "${STROOM_NODE_ROLE}" == 'Frontend' ]]; then
  enabled='false'
fi
call_api job/v1/setJobsEnabled/"${STROOM_NODE}" -X PUT -d "{ \"enabled\": $enabled }"
if [[ $enabled == 'true' ]]; then
  log "Node ${STROOM_NODE} jobs enabled" $log_file
else
  log "Node ${STROOM_NODE} jobs disabled (as this is a dedicated UI node)" $log_file
fi