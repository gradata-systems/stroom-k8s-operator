This is a Kubernetes operator written in Go and developed using the [Operator SDK](https://sdk.operatorframework.io).

Its purpose is to simplify the deployment and operational management of a Stroom cluster in a Kubernetes environment.

# Features
## Deployment
1. Custom Resource Definitions (CRDs) for defining the desired state of a Stroom cluster, nodes and database
1. Ability to designate dedicated `Processing` and `Frontend` nodes and route event traffic appropriately
1. Automatic secrets management (e.g. secure database credential generation and storage)
   
## Operations
1. Scheduled database backups
1. Stroom node audit log shipping
1. Stroom node lifecycle management
    1. Prevent node shutdown while Stroom processing tasks are still active
    1. Automatic task draining during shutdown
    1. Rolling Stroom version upgrades
1. Automatically scale the maximum tasks for each Stroom node by continually assessing average CPU usage.
   The following parameters are configurable:
    1. Adjustment time interval (how often adjustments should be made)
    1. Metric sliding window (calculate the average based on the specified number of minutes)
    1. Minimum CPU % to keep the node above
    1. Maximum CPU % to keep the node below
    1. Minimum number of tasks allowed for the node
    1. Maximum number of tasks allowed for the node
    
# Installation
1. Install [Operator Lifecycle Manager](https://olm.operatorframework.io/docs/getting-started/)
1. Create a CatalogSource
1. Create a Subscription
1. Deploy Stroom cluster resources. Options:
    1. Helm chart (preferred)
    1. Manually via kubectl