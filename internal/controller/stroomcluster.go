package controller

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	AppHttpPortName              = "app-http"
	AppHttpPortNumber            = 8080
	AppHttpsPortName             = "app-https"
	AppHttpsPortNumber           = 8443
	AdminPortName                = "admin"
	AdminPortNumber              = 8081
	HealthCheckPath              = "/stroomAdmin/filteredhealthcheck"
	DefaultProbePeriodSeconds    = 10
	DefaultProbeFailureThreshold = 3
	StaticContentVolumeName      = "static-content"
	StroomNodePvcName            = "data"
	StroomNodeContainerName      = "stroom-node"
	StroomKeystoreVolumeName     = "keystore"
	StroomTlsVolumeName          = "tls"
	StroomApiTokenVolumeName     = "api-token"
	StroomApiTokenMountPath      = "/stroom/auth"
	LogSenderConfigMapName       = "log-sender-configmap"
	LogSenderDefaultCpuLimit     = "500m"
	LogSenderDefaultMemoryLimit  = "256Mi"
	LogSenderCertsVolumeName     = "log-sender-certs"
)

func (r *StroomClusterReconciler) createNodeSetPvcLabels(stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet) map[string]string {
	labels := stroomCluster.GetLabels()
	labels[stroomv1.NodeSetLabel] = nodeSet.Name

	return labels
}

func (r *StroomClusterReconciler) createServiceAccount(stroomCluster *stroomv1.StroomCluster) *corev1.ServiceAccount {
	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stroomCluster.GetBaseName(),
			Namespace: stroomCluster.Namespace,
			Labels:    stroomCluster.GetLabels(),
		},
	}

	ctrl.SetControllerReference(stroomCluster, &serviceAccount, r.Scheme)
	return &serviceAccount
}

func (r *StroomClusterReconciler) createConfigMap(stroomCluster *stroomv1.StroomCluster, data map[string]string) *corev1.ConfigMap {
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stroomCluster.GetStaticContentConfigMapName(),
			Namespace: stroomCluster.Namespace,
			Labels:    stroomCluster.GetLabels(),
		},
		Data: data,
	}

	ctrl.SetControllerReference(stroomCluster, &configMap, r.Scheme)
	return &configMap
}

func (r *StroomClusterReconciler) createLogSenderConfigMap(stroomCluster *stroomv1.StroomCluster) *corev1.ConfigMap {
	var logSender = stroomCluster.Spec.LogSender

	// Configure log sender with or without client certificate authentication
	var logSenderConfig map[string]string

	if !logSender.Tls.IsZero() {
		logSenderConfig = map[string]string{
			"crontab.txt": "" +
				"* * * * * ${LOG_SENDER_SCRIPT} \"${STROOM_BASE_LOGS_DIR}/access\" STROOM-ACCESS-EVENTS \"${STROOM_DATAFEED_URL}\" --system \"${STROOM_SYSTEM_NAME}\" --environment \"${STROOM_ENVIRONMENT_NAME}\" --file-regex \"${STROOM_FILE_REGEX}\" -m ${STROOM_MAX_DELAY_SECS} --delete-after-sending --cacert /stroom-log-sender/certs/ca.crt --cert /stroom-log-sender/certs/tls.crt --key /stroom-log-sender/certs/tls.key --compress > /dev/stdout\n" +
				"* * * * * ${LOG_SENDER_SCRIPT} \"${STROOM_BASE_LOGS_DIR}/app\"    STROOM-APP-EVENTS    \"${STROOM_DATAFEED_URL}\" --system \"${STROOM_SYSTEM_NAME}\" --environment \"${STROOM_ENVIRONMENT_NAME}\" --file-regex \"${STROOM_FILE_REGEX}\" -m ${STROOM_MAX_DELAY_SECS} --delete-after-sending --cacert /stroom-log-sender/certs/ca.crt --cert /stroom-log-sender/certs/tls.crt --key /stroom-log-sender/certs/tls.key --compress > /dev/stdout\n" +
				"* * * * * ${LOG_SENDER_SCRIPT} \"${STROOM_BASE_LOGS_DIR}/user\"   STROOM-USER-EVENTS   \"${STROOM_DATAFEED_URL}\" --system \"${STROOM_SYSTEM_NAME}\" --environment \"${STROOM_ENVIRONMENT_NAME}\" --file-regex \"${STROOM_FILE_REGEX}\" -m ${STROOM_MAX_DELAY_SECS} --delete-after-sending --cacert /stroom-log-sender/certs/ca.crt --cert /stroom-log-sender/certs/tls.crt --key /stroom-log-sender/certs/tls.key --compress > /dev/stdout",
		}
	} else {
		logSenderConfig = map[string]string{
			"crontab.txt": "" +
				"* * * * * ${LOG_SENDER_SCRIPT} \"${STROOM_BASE_LOGS_DIR}/access\" STROOM-ACCESS-EVENTS \"${STROOM_DATAFEED_URL}\" --system \"${STROOM_SYSTEM_NAME}\" --environment \"${STROOM_ENVIRONMENT_NAME}\" --file-regex \"${STROOM_FILE_REGEX}\" -m ${STROOM_MAX_DELAY_SECS} --delete-after-sending --no-secure --compress > /dev/stdout\n" +
				"* * * * * ${LOG_SENDER_SCRIPT} \"${STROOM_BASE_LOGS_DIR}/app\"    STROOM-APP-EVENTS    \"${STROOM_DATAFEED_URL}\" --system \"${STROOM_SYSTEM_NAME}\" --environment \"${STROOM_ENVIRONMENT_NAME}\" --file-regex \"${STROOM_FILE_REGEX}\" -m ${STROOM_MAX_DELAY_SECS} --delete-after-sending --no-secure --compress > /dev/stdout\n" +
				"* * * * * ${LOG_SENDER_SCRIPT} \"${STROOM_BASE_LOGS_DIR}/user\"   STROOM-USER-EVENTS   \"${STROOM_DATAFEED_URL}\" --system \"${STROOM_SYSTEM_NAME}\" --environment \"${STROOM_ENVIRONMENT_NAME}\" --file-regex \"${STROOM_FILE_REGEX}\" -m ${STROOM_MAX_DELAY_SECS} --delete-after-sending --no-secure --compress > /dev/stdout",
		}
	}

	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stroomCluster.GetLogSenderConfigMapName(),
			Namespace: stroomCluster.Namespace,
			Labels:    stroomCluster.GetLabels(),
		},
		Data: logSenderConfig,
	}

	ctrl.SetControllerReference(stroomCluster, &configMap, r.Scheme)
	return &configMap
}

func (r *StroomClusterReconciler) createStaticContentVolume(stroomCluster *stroomv1.StroomCluster) *corev1.Volume {
	// Scripts need execute permissions
	var fileMode int32 = 0554
	return &corev1.Volume{
		Name: StaticContentVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: stroomCluster.GetStaticContentConfigMapName(),
				},
				DefaultMode: &fileMode,
			},
		},
	}
}

func (r *StroomClusterReconciler) createStatefulSet(stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet, dbInfo *DatabaseConnectionInfo) *appsv1.StatefulSet {
	logSender := stroomCluster.Spec.LogSender

	env := append([]corev1.EnvVar{{
		Name:  "API_GATEWAY_HOST",
		Value: stroomCluster.Spec.Ingress.HostName,
	}, {
		Name:  "ADMIN_CONTEXT_PATH",
		Value: "/stroomAdmin",
	}, {
		Name:  "APPLICATION_CONTEXT_PATH",
		Value: "/",
	}, {
		Name: "DOCKER_HOST_HOSTNAME",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.labels['statefulset.kubernetes.io/pod-name']",
			},
		},
	}, {
		Name: "DOCKER_HOST_IP",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "status.podIP",
			},
		},
	}, {
		Name: "POD_HOSTNAME",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.labels['statefulset.kubernetes.io/pod-name']",
			},
		},
	}, {
		Name:  "POD_SUBDOMAIN",
		Value: fmt.Sprintf("%v.%v.svc.cluster.local", stroomCluster.GetNodeSetHeadlessServiceName(nodeSet), stroomCluster.Namespace),
	}, {
		Name:  "JAVA_OPTS",
		Value: r.getJvmOptions(stroomCluster, nodeSet),
	}, {
		Name:  "STROOM_APP_HTTP_PORT",
		Value: strconv.Itoa(AppHttpPortNumber),
	}, {
		Name:  "STROOM_APP_HTTPS_PORT",
		Value: strconv.Itoa(AppHttpsPortNumber),
	}, {
		Name:  "STROOM_ADMIN_PORT",
		Value: strconv.Itoa(AdminPortNumber),
	}, {
		Name: "STROOM_NODE",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "metadata.labels['statefulset.kubernetes.io/pod-name']",
			},
		},
	}, {
		Name:  "STROOM_NODE_ROLE",
		Value: string(nodeSet.Role),
	}, {
		Name:  "STROOM_JDBC_DRIVER_URL",
		Value: dbInfo.ToJdbcConnectionString(stroomCluster.Spec.AppDatabaseName),
	}, {
		Name:  "STROOM_JDBC_DRIVER_CLASS_NAME",
		Value: "com.mysql.cj.jdbc.Driver",
	}, {
		Name:  "STROOM_JDBC_DRIVER_USERNAME",
		Value: dbInfo.UserName,
	}, {
		Name: "STROOM_JDBC_DRIVER_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: dbInfo.SecretName,
				},
				Key: dbInfo.UserName,
			},
		},
	}, {
		Name:  "STROOM_STATISTICS_JDBC_DRIVER_URL",
		Value: dbInfo.ToJdbcConnectionString(stroomCluster.Spec.StatsDatabaseName),
	}, {
		Name:  "STROOM_STATISTICS_JDBC_DRIVER_CLASS_NAME",
		Value: "com.mysql.cj.jdbc.Driver",
	}, {
		Name:  "STROOM_STATISTICS_JDBC_DRIVER_USERNAME",
		Value: dbInfo.UserName,
	}, {
		Name: "STROOM_STATISTICS_JDBC_DRIVER_PASSWORD",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: dbInfo.SecretName,
				},
				Key: dbInfo.UserName,
			},
		},
	}, {
		Name:  "STROOM_TEMP",
		Value: "/stroom/tmp",
	}}, stroomCluster.Spec.ExtraEnv...)

	// List the jobs that should be managed by the node startup and shutdown scripts.
	// These are typically node role dependent. Frontend nodes for instance, shouldn't have the `Data Processor` node
	// enabled.
	var includeJobs []string
	if nodeSet.ManagedJobs != nil && len(nodeSet.ManagedJobs) > 0 {
		includeJobs = nodeSet.ManagedJobs
	} else if nodeSet.Role == stroomv1.ProcessingNodeRole {
		includeJobs = []string{}
	} else if nodeSet.Role == stroomv1.FrontendNodeRole {
		includeJobs = []string{
			"Reindex Content",
		}
	}
	env = append(env, corev1.EnvVar{
		Name:  "STROOM_NODE_MANAGED_JOBS",
		Value: strings.Join(includeJobs, ","),
	})

	if !stroomCluster.Spec.Https.IsZero() {
		env = append(env, corev1.EnvVar{
			Name: "STROOM_KEYSTORE_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: stroomCluster.Spec.Https.TlsKeystorePasswordSecretRef.SecretName,
					},
					Key: stroomCluster.Spec.Https.TlsKeystorePasswordSecretRef.Key,
				},
			},
		})
	}

	// If OpenID configuration is defined, pass the OpenID client ID and secret as environment variables
	openIdConfig := stroomCluster.Spec.OpenId
	if !openIdConfig.IsZero() {
		env = append(env, []corev1.EnvVar{
			{
				Name:  "STROOM_OPERATOR_OPENID_CLIENT_ID",
				Value: openIdConfig.ClientId,
			},
			{
				Name: "STROOM_OPERATOR_OPENID_CLIENT_SECRET",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: openIdConfig.ClientSecret.SecretName,
						},
						Key: openIdConfig.ClientSecret.Key,
					},
				},
			},
		}...)
	}

	volumes := []corev1.Volume{
		*r.createStaticContentVolume(stroomCluster),
		{
			Name: StroomApiTokenVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	if !stroomCluster.Spec.Https.IsZero() {
		volumes = append(volumes, []corev1.Volume{
			{
				Name: StroomTlsVolumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: stroomCluster.Spec.Https.TlsSecretName,
					},
				},
			},
			{
				Name: StroomKeystoreVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		}...)
	}

	if !logSender.IsZero() {
		volumes = append(volumes, corev1.Volume{
			Name: LogSenderConfigMapName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: stroomCluster.GetLogSenderConfigMapName(),
					},
				},
			},
		})

		if !logSender.Tls.IsZero() {
			volumes = append(volumes, corev1.Volume{
				Name: LogSenderCertsVolumeName,
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: stroomCluster.Spec.LogSender.Tls.SecretName,
					},
				},
			})
		}
	}

	// If a Stroom node config override is provided, create a volume for it
	if !stroomCluster.Spec.ConfigMapRef.IsZero() {
		volumes = append(volumes, corev1.Volume{
			Name: "config-override",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: stroomCluster.Spec.ConfigMapRef.Name,
					},
				},
			},
		})
	}

	volumeMounts := []corev1.VolumeMount{{
		Name:      StaticContentVolumeName,
		SubPath:   "utils.sh",
		MountPath: "/stroom/scripts/utils.sh",
		ReadOnly:  true,
	}, {
		Name:      StaticContentVolumeName,
		SubPath:   "node-start.sh",
		MountPath: "/stroom/scripts/node-start.sh",
		ReadOnly:  true,
	}, {
		Name:      StaticContentVolumeName,
		SubPath:   "node-pre-stop.sh",
		MountPath: "/stroom/scripts/node-pre-stop.sh",
		ReadOnly:  true,
	}, {
		Name:      StroomApiTokenVolumeName,
		MountPath: StroomApiTokenMountPath,
	}, {
		Name:      StroomNodePvcName,
		SubPath:   "logs",
		MountPath: "/stroom/logs",
	}, {
		Name:      StroomNodePvcName,
		SubPath:   "output",
		MountPath: "/stroom/output",
	}, {
		Name:      StroomNodePvcName,
		SubPath:   "tmp",
		MountPath: "/stroom/tmp",
	}, {
		Name:      StroomNodePvcName,
		SubPath:   "lmdb",
		MountPath: "/stroom/lmdb",
	}, {
		Name:      StroomNodePvcName,
		SubPath:   "lmdb_library",
		MountPath: "/stroom/lmdb_library",
	}, {
		Name:      StroomNodePvcName,
		SubPath:   "planb",
		MountPath: "/stroom/planb",
	}, {
		Name:      StroomNodePvcName,
		SubPath:   "proxy-repo",
		MountPath: "/stroom/proxy_repo",
	}, {
		Name:      StroomNodePvcName,
		SubPath:   "reference-data",
		MountPath: "/stroom/reference_data",
	}, {
		Name:      StroomNodePvcName,
		SubPath:   "reference_staging_data",
		MountPath: "/stroom/reference_staging_data",
	}, {
		Name:      StroomNodePvcName,
		SubPath:   "search-results",
		MountPath: "/stroom/search_results",
	}}

	if !stroomCluster.Spec.Https.IsZero() {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      StroomKeystoreVolumeName,
			MountPath: "/stroom/pki/tls",
			ReadOnly:  true,
		})
	}

	r.appendConfigVolumeMounts(stroomCluster, &volumeMounts)

	if len(stroomCluster.Spec.ExtraVolumes) > 0 {
		volumes = append(volumes, stroomCluster.Spec.ExtraVolumes...)
	}
	if len(stroomCluster.Spec.ExtraVolumeMounts) > 0 {
		volumeMounts = append(volumeMounts, stroomCluster.Spec.ExtraVolumeMounts...)
	}
	if len(nodeSet.ExtraVolumeMounts) > 0 {
		volumeMounts = append(volumeMounts, nodeSet.ExtraVolumeMounts...)
	}

	var initContainers []corev1.Container
	if !stroomCluster.Spec.Https.IsZero() {
		initContainers = append(initContainers, corev1.Container{
			Name:            "generate-keystore",
			Image:           stroomCluster.Spec.Image.String(),
			ImagePullPolicy: stroomCluster.Spec.ImagePullPolicy,
			Command: []string{
				"sh",
				"-c",
				"/opt/scripts/generate-keystore.sh",
			},
			Env: []corev1.EnvVar{{
				Name: "STROOM_KEYSTORE_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: stroomCluster.Spec.Https.TlsKeystorePasswordSecretRef.SecretName,
						},
						Key: stroomCluster.Spec.Https.TlsKeystorePasswordSecretRef.Key,
					},
				},
			}},
			VolumeMounts: []corev1.VolumeMount{{
				Name:      StaticContentVolumeName,
				SubPath:   "generate-keystore.sh",
				MountPath: "/opt/scripts/generate-keystore.sh",
				ReadOnly:  true,
			}, {
				Name:      StroomTlsVolumeName,
				MountPath: "/opt/tls",
				ReadOnly:  true,
			}, {
				Name:      StroomKeystoreVolumeName,
				MountPath: "/data",
			}},
		},
		)
	}

	containers := []corev1.Container{{
		Name:            StroomNodeContainerName,
		Image:           stroomCluster.Spec.Image.String(),
		ImagePullPolicy: stroomCluster.Spec.ImagePullPolicy,
		Env:             env,
		VolumeMounts:    volumeMounts,
		SecurityContext: &nodeSet.SecurityContext,
		Ports: []corev1.ContainerPort{{
			Name:          AppHttpPortName,
			ContainerPort: AppHttpPortNumber,
			Protocol:      corev1.ProtocolTCP,
		}, {
			Name:          AppHttpsPortName,
			ContainerPort: AppHttpsPortNumber,
			Protocol:      corev1.ProtocolTCP,
		}, {
			Name:          AdminPortName,
			ContainerPort: AdminPortNumber,
			Protocol:      corev1.ProtocolTCP,
		}},
		StartupProbe:   createStartupProbe(stroomCluster, nodeSet),
		ReadinessProbe: createHealthCheckProbe(&nodeSet.ReadinessProbeTimings.ProbeTimings, derefSlice(nodeSet.ReadinessProbeTimings.Allow), derefSlice(nodeSet.ReadinessProbeTimings.Deny), AdminPortName),
		LivenessProbe:  createHealthCheckProbe(&nodeSet.LivenessProbeTimings.ProbeTimings, derefSlice(nodeSet.LivenessProbeTimings.Allow), derefSlice(nodeSet.LivenessProbeTimings.Deny), AdminPortName),
		Resources:      nodeSet.Resources,
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"bash",
						"/stroom/scripts/node-pre-stop.sh",
					},
				},
			},
		},
	}}

	if !logSender.IsZero() {
		containers = append(containers, r.createLogSenderContainer(stroomCluster))
	}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stroomCluster.GetNodeSetName(nodeSet),
			Namespace: stroomCluster.Namespace,
			Labels:    stroomCluster.GetLabels(),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:            &nodeSet.Count,
			PodManagementPolicy: stroomCluster.Spec.PodManagementPolicy,
			ServiceName:         stroomCluster.GetNodeSetHeadlessServiceName(nodeSet),
			Selector: &metav1.LabelSelector{
				MatchLabels: stroomCluster.GetNodeSetSelectorLabels(nodeSet),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nodeSet.PodAnnotations,
					Labels:      stroomCluster.GetNodeSetSelectorLabels(nodeSet),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName:            stroomCluster.GetBaseName(),
					SecurityContext:               &nodeSet.PodSecurityContext,
					TerminationGracePeriodSeconds: &stroomCluster.Spec.NodeTerminationPeriodSecs,
					InitContainers:                initContainers,
					Containers:                    containers,
					Volumes:                       volumes,
					NodeSelector:                  nodeSet.NodeSelector,
					Affinity:                      &nodeSet.Affinity,
					Tolerations:                   nodeSet.Tolerations,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name:   StroomNodePvcName,
					Labels: r.createNodeSetPvcLabels(stroomCluster, nodeSet),
				},
				Spec: nodeSet.LocalDataVolumeClaim,
			}},
		},
	}

	ctrl.SetControllerReference(stroomCluster, statefulSet, r.Scheme)
	return statefulSet
}

func (r *StroomClusterReconciler) createLogSenderContainer(stroomCluster *stroomv1.StroomCluster) corev1.Container {
	logSender := stroomCluster.Spec.LogSender

	destinationUrl := logSender.DestinationUrl
	if destinationUrl == "" {
		destinationUrl = stroomCluster.GetDatafeedUrl()
	}

	environmentName := logSender.EnvironmentName
	if environmentName == "" {
		environmentName = strings.ToUpper(stroomCluster.Name)
	}

	systemName := logSender.SystemName
	if systemName == "" {
		systemName = "Stroom"
	}

	// Set default resource limits if not specified
	resources := logSender.Resources
	if resources.Size() == 0 {
		resources = corev1.ResourceRequirements{
			Limits: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceCPU:    resource.MustParse(LogSenderDefaultCpuLimit),
				corev1.ResourceMemory: resource.MustParse(LogSenderDefaultMemoryLimit),
			},
		}
	}

	logSenderVolumeMounts := []corev1.VolumeMount{
		{
			Name:      StroomNodePvcName,
			SubPath:   "logs",
			MountPath: "/stroom-log-sender/log-volumes/stroom",
		}, {
			Name:      LogSenderConfigMapName,
			MountPath: "/stroom-log-sender/config",
			ReadOnly:  true,
		},
	}

	if !logSender.Tls.IsZero() {
		logSenderVolumeMounts = append(logSenderVolumeMounts, corev1.VolumeMount{
			Name:      LogSenderCertsVolumeName,
			MountPath: "/stroom-log-sender/certs",
			ReadOnly:  true,
		})
	}

	return corev1.Container{
		Name:            "log-sender",
		Image:           logSender.Image.String(),
		ImagePullPolicy: logSender.ImagePullPolicy,
		SecurityContext: &logSender.SecurityContext,
		Env: []corev1.EnvVar{{
			Name:  "LOG_SENDER_SCRIPT",
			Value: "/stroom-log-sender/send_to_stroom.sh",
		}, {
			Name:  "STROOM_DATAFEED_URL",
			Value: destinationUrl,
		}, {
			Name:  "STROOM_BASE_LOGS_DIR",
			Value: "/stroom-log-sender/log-volumes/stroom",
		}, {
			Name:  "STROOM_FILE_REGEX",
			Value: `.*/[a-z]+-[0-9]+-[0-9]+-[0-9]+T[0-9]+:[0-9]+\.log(\.gz)?$`,
		}, {
			Name:  "STROOM_ENVIRONMENT_NAME",
			Value: environmentName,
		}, {
			Name:  "STROOM_SYSTEM_NAME",
			Value: systemName,
		}, {
			Name:  "STROOM_MAX_DELAY_SECS",
			Value: "15",
		}},
		VolumeMounts: logSenderVolumeMounts,
		Resources:    resources,
	}
}

func (r *StroomClusterReconciler) appendConfigVolumeMounts(stroomCluster *stroomv1.StroomCluster, volumeMounts *[]corev1.VolumeMount) {
	// If a Stroom node config override is provided, mount the existing ConfigMap
	const configMountPath = "/stroom/config/config.yml"
	if !stroomCluster.Spec.ConfigMapRef.IsZero() {
		*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
			Name:      "config-override",
			SubPath:   stroomCluster.Spec.ConfigMapRef.ItemName,
			MountPath: configMountPath,
			ReadOnly:  true,
		})
	} else {
		// Use the default config
		*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
			Name:      StaticContentVolumeName,
			SubPath:   "stroomcluster-config.yaml",
			MountPath: configMountPath,
			ReadOnly:  true,
		})
	}
}

// getJvmOptions generates JVM options from the requested NodeSet resources.
// The Java heap (min/max) is set to half the max. allocated memory, or 30Gi, whichever is smaller.
// The 30Gi limit ensures that the JVM uses compressed memory pointers.
func (r *StroomClusterReconciler) getJvmOptions(stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet) string {
	// If JAVA_OPTS was specified in `ExtraEnv`, use that value
	for _, env := range stroomCluster.Spec.ExtraEnv {
		if env.Name == "JAVA_OPTS" {
			return env.Value
		}
	}

	var jvmOpts []string

	if nodeSet.MemoryOptions.InitialPercentage > 0 {
		jvmOpts = append(jvmOpts, fmt.Sprintf("-XX:InitialRAMPercentage=%v", nodeSet.MemoryOptions.InitialPercentage))
	}
	if nodeSet.MemoryOptions.MaxPercentage > 0 {
		jvmOpts = append(jvmOpts, fmt.Sprintf("-XX:MaxRAMPercentage=%v", nodeSet.MemoryOptions.MaxPercentage))
	}

	if len(stroomCluster.Spec.ExtraJvmOpts) > 0 {
		jvmOpts = append(jvmOpts, stroomCluster.Spec.ExtraJvmOpts...)
	}

	return strings.Join(jvmOpts, " ")
}

// createStartupProbe creates a probe that runs the node startup script. When nodes in the NodeSet start in parallel,
// the time allowed is extended by LockWaitAllowanceSeconds for each additional node, as nodes wait on the Stroom
// bootstrap lock while another node upgrades the database.
func createStartupProbe(stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet) *corev1.Probe {
	timings := nodeSet.StartupProbeTimings
	failureThreshold := timings.FailureThreshold
	if stroomCluster.Spec.PodManagementPolicy == appsv1.ParallelPodManagement && nodeSet.Count > 1 && timings.LockWaitAllowanceSeconds > 0 {
		periodSeconds := timings.PeriodSeconds
		if periodSeconds <= 0 {
			periodSeconds = DefaultProbePeriodSeconds
		}
		if failureThreshold <= 0 {
			failureThreshold = DefaultProbeFailureThreshold
		}
		allowanceSeconds := int64(timings.LockWaitAllowanceSeconds) * int64(nodeSet.Count-1)
		extraFailures := (allowanceSeconds + int64(periodSeconds) - 1) / int64(periodSeconds)
		failureThreshold += int32(min(extraFailures, int64(math.MaxInt32-failureThreshold)))
	}

	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"bash",
					"/stroom/scripts/node-start.sh",
				},
			},
		},
		InitialDelaySeconds: timings.InitialDelaySeconds,
		PeriodSeconds:       timings.PeriodSeconds,
		TimeoutSeconds:      timings.TimeoutSeconds,
		SuccessThreshold:    timings.SuccessThreshold,
		FailureThreshold:    failureThreshold,
	}
}

// createHealthCheckProbe creates a probe that queries the Stroom filtered health check endpoint, running only the
// health checks permitted by the allow and deny lists
func createHealthCheckProbe(probeTimings *stroomv1.ProbeTimings, allow []string, deny []string, portName string) *corev1.Probe {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: healthCheckPath(allow, deny),
				Port: intstr.IntOrString{
					Type:   intstr.String,
					StrVal: portName,
				},
			},
		},
	}

	if probeTimings.InitialDelaySeconds != 0 {
		probe.InitialDelaySeconds = probeTimings.InitialDelaySeconds
	}
	if probeTimings.PeriodSeconds != 0 {
		probe.PeriodSeconds = probeTimings.PeriodSeconds
	}
	if probeTimings.TimeoutSeconds != 0 {
		probe.TimeoutSeconds = probeTimings.TimeoutSeconds
	}
	if probeTimings.SuccessThreshold != 0 {
		probe.SuccessThreshold = probeTimings.SuccessThreshold
	}
	if probeTimings.FailureThreshold != 0 {
		probe.FailureThreshold = probeTimings.FailureThreshold
	}

	return probe
}

// derefSlice returns the slice a pointer refers to, or nil
func derefSlice(slice *[]string) []string {
	if slice == nil {
		return nil
	}
	return *slice
}

// healthCheckPath returns the path of the Stroom filtered health check endpoint, with query parameters that filter
// which health checks are run. Stroom expects each list as a single comma-delimited parameter and doesn't trim
// whitespace, so names are trimmed and blank names omitted.
func healthCheckPath(allow []string, deny []string) string {
	query := url.Values{}
	for name, checks := range map[string][]string{"allow": allow, "deny": deny} {
		var names []string
		for _, check := range checks {
			if check = strings.TrimSpace(check); check != "" {
				names = append(names, check)
			}
		}
		if len(names) > 0 {
			query.Set(name, strings.Join(names, ","))
		}
	}

	if len(query) == 0 {
		return HealthCheckPath
	}
	return HealthCheckPath + "?" + query.Encode()
}

func (r *StroomClusterReconciler) createService(stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet, name string, clusterIp string) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: stroomCluster.Namespace,
			Labels:    stroomCluster.GetLabels(),
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: clusterIp,
			Selector:  stroomCluster.GetNodeSetSelectorLabels(nodeSet),
			Ports: []corev1.ServicePort{{
				Name:     AppHttpsPortName,
				Port:     AppHttpsPortNumber,
				Protocol: corev1.ProtocolTCP,
			}, {
				Name:     AppHttpPortName,
				Port:     AppHttpPortNumber,
				Protocol: corev1.ProtocolTCP,
			}, {
				Name:     AdminPortName,
				Port:     AdminPortNumber,
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}

	ctrl.SetControllerReference(stroomCluster, service, r.Scheme)
	return service
}

func (r *StroomClusterReconciler) createIngresses(ctx context.Context, stroomCluster *stroomv1.StroomCluster) []netv1.Ingress {
	logger := log.FromContext(ctx)
	ingressSettings := stroomCluster.Spec.Ingress
	var ingresses []netv1.Ingress

	// Find out the first non-UI NodeSet, so we know where to route datafeed traffic to
	firstNonUiServiceName := ""
	for _, nodeSet := range stroomCluster.Spec.NodeSets {
		if nodeSet.Role != stroomv1.FrontendNodeRole {
			firstNonUiServiceName = stroomCluster.GetNodeSetServiceName(&nodeSet)
			break
		}
	}

	// Settings shared by every Ingress. Requests are sent to the nodes over HTTPS if it's enabled. TLS between
	// clients and the Ingress controller uses the Ingress Secret, if one is specified.
	nginxBackendProtocol := "HTTP"
	appPortName := AppHttpPortName
	if !stroomCluster.Spec.Https.IsZero() {
		nginxBackendProtocol = "HTTPS"
		appPortName = AppHttpsPortName
	}
	var ingressTls []netv1.IngressTLS
	if ingressSettings.SecretName != "" {
		ingressTls = []netv1.IngressTLS{{
			Hosts:      []string{ingressSettings.HostName},
			SecretName: ingressSettings.SecretName,
		}}
	}
	var ingressClassName *string
	if ingressSettings.ClassName != "" {
		ingressClassName = &ingressSettings.ClassName
	}

	// Create an Ingress for each route in each NodeSet, where Ingress is enabled
	for _, nodeSet := range stroomCluster.Spec.NodeSets {
		clusterName := stroomCluster.GetBaseName()
		serviceName := stroomCluster.GetNodeSetServiceName(&nodeSet)

		if nodeSet.IngressEnabled != nil && !*nodeSet.IngressEnabled {
			continue
		}

		if nodeSet.Role != stroomv1.ProcessingNodeRole {
			ingressAnnotations := map[string]string{
				"nginx.ingress.kubernetes.io/backend-protocol": nginxBackendProtocol,
				"nginx.ingress.kubernetes.io/affinity":         "cookie",
				"nginx.ingress.kubernetes.io/affinity-mode":    "persistent",
				"nginx.ingress.kubernetes.io/proxy-body-size":  "0", // Disable client request payload size checking
			}

			// Apply any user-provided annotations
			for k, v := range nodeSet.IngressAnnotations {
				ingressAnnotations[k] = v
			}

			// Apply any user-provided ingress labels
			ingressLabels := stroomCluster.GetLabels()
			for k, v := range nodeSet.IngressLabels {
				ingressLabels[k] = v
			}

			ingresses = append(ingresses,
				netv1.Ingress{
					// Default route (/)
					ObjectMeta: metav1.ObjectMeta{
						Name:        clusterName,
						Namespace:   stroomCluster.Namespace,
						Labels:      ingressLabels,
						Annotations: ingressAnnotations,
					},
					Spec: netv1.IngressSpec{
						IngressClassName: ingressClassName,
						TLS:              ingressTls,
						Rules: []netv1.IngressRule{
							// Explicitly route datafeed traffic to the first non-UI NodeSet
							r.createIngressRule(ingressSettings.HostName, netv1.PathTypeExact, "/stroom/noauth/datafeed", firstNonUiServiceName, appPortName, ingressSettings.PathTypeOverride),

							// All other traffic is routed to the UI NodeSets
							r.createIngressRule(ingressSettings.HostName, netv1.PathTypePrefix, "/", serviceName, appPortName, ingressSettings.PathTypeOverride),
						},
					},
				})
		}

		if nodeSet.Role != stroomv1.FrontendNodeRole {
			ingressAnnotations := map[string]string{
				"nginx.ingress.kubernetes.io/backend-protocol": nginxBackendProtocol,
				"nginx.ingress.kubernetes.io/rewrite-target":   "/stroom/noauth/datafeed",
				"nginx.ingress.kubernetes.io/proxy-body-size":  "0", // Disable client request payload size checking
				"haproxy.router.openshift.io/rewrite-target":   "/stroom/noauth/datafeed",
			}

			// Apply any user-provided annotations
			for k, v := range nodeSet.IngressAnnotations {
				ingressAnnotations[k] = v
			}

			//Apply any user-provided ingress labels
			ingressLabels := stroomCluster.GetLabels()
			for k, v := range nodeSet.IngressLabels {
				ingressLabels[k] = v
			}

			ingresses = append(ingresses, netv1.Ingress{
				// Rewrite requests to `/stroom/datafeeddirect` to `/stroom/noauth/datafeed`
				ObjectMeta: metav1.ObjectMeta{
					Name:        clusterName + "-datafeed",
					Namespace:   stroomCluster.Namespace,
					Labels:      ingressLabels,
					Annotations: ingressAnnotations,
				},
				Spec: netv1.IngressSpec{
					IngressClassName: ingressClassName,
					TLS:              ingressTls,
					Rules: []netv1.IngressRule{
						r.createIngressRule(ingressSettings.HostName, netv1.PathTypeExact, "/stroom/datafeeddirect", serviceName, appPortName, ingressSettings.PathTypeOverride),
					},
				},
			})
		}
	}

	for _, ingress := range ingresses {
		if err := ctrl.SetControllerReference(stroomCluster, &ingress, r.Scheme); err != nil {
			logger.Error(err, fmt.Sprintf("Could not set controller reference on ingress '%v/%v'", ingress.Namespace, ingress.Name))
		}
	}

	return ingresses
}

func (r *StroomClusterReconciler) createIngressRule(hostName string, pathType netv1.PathType, path string, serviceName string, appPortName string, pathTypeOverride bool) netv1.IngressRule {

	var actualPathtype netv1.PathType
	if pathTypeOverride {
		actualPathtype = netv1.PathTypeImplementationSpecific
	} else {
		actualPathtype = pathType
	}

	return netv1.IngressRule{
		Host: hostName,
		IngressRuleValue: netv1.IngressRuleValue{
			HTTP: &netv1.HTTPIngressRuleValue{
				Paths: []netv1.HTTPIngressPath{{
					Path:     path,
					PathType: &actualPathtype,
					Backend: netv1.IngressBackend{
						Service: &netv1.IngressServiceBackend{
							Name: serviceName,
							Port: netv1.ServiceBackendPort{
								Name: appPortName,
							},
						},
					},
				}},
			},
		},
	}
}
