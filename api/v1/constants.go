package v1

const (
	StroomClusterFinalizerName = "stroomcluster.finalizers.stroom.gchq.github.io"
	WaitNodeTasksFinalizerName = "waitnodetasks.finalizers.stroom.gchq.github.io"
	StroomOperatorUserId       = "K8S_OPERATOR"

	// SecretFileMode is the file mode to use for Secret volume mounts
	SecretFileMode int32 = 0400
)

type UserPermission string

const (
	UserPermissionManageNodes UserPermission = "Manage Nodes"
	UserPermissionManageJobs  UserPermission = "Manage Jobs"
	UserPermissionManageTasks UserPermission = "Manage Tasks"
)
