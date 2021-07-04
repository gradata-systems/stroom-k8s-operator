package v1

const (
	StroomClusterFinalizerName = "stroomcluster.finalizers.stroom.gchq.github.io"
	WaitNodeTasksFinalizerName = "waitnodetasks.finalizers.stroom.gchq.github.io"
	StroomInternalUserName     = "INTERNAL_PROCESSING_USER"
	TaskCountApi               = "/api/task/v1/noauth/count"

	// SecretFileMode is the file mode to use for Secret volume mounts
	SecretFileMode int32 = 0400
)
