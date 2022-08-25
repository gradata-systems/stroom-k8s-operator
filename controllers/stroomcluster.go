package controllers

import (
	"context"
	_ "embed"
	"fmt"
	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
	"strings"
	"time"
)

const (
	AppPortName                 = "app"
	AppPortNumber               = 8080
	AdminPortName               = "admin"
	AdminPortNumber             = 8081
	StroomNodeApiKeyPath        = "/stroom/auth/api_key"
	StroomNodePvcName           = "data"
	StroomNodeContainerName     = "stroom-node"
	StroomCliJobRequeueInterval = time.Second * 10
	LogSenderDefaultCpuLimit    = "500m"
	LogSenderDefaultMemoryLimit = "256Mi"
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

func (r *StroomClusterReconciler) createSecret(stroomCluster *stroomv1.StroomCluster, apiKey string) *corev1.Secret {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stroomCluster.GetBaseName(),
			Namespace: stroomCluster.Namespace,
			Labels:    stroomCluster.GetLabels(),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"api_key": []byte(apiKey),
		},
	}

	// Do not set the controller reference, as we want the Secret to persist if the StroomCluster is deleted

	return &secret
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
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stroomCluster.GetLogSenderConfigMapName(),
			Namespace: stroomCluster.Namespace,
			Labels:    stroomCluster.GetLabels(),
		},
		Data: map[string]string{
			"crontab.txt": "" +
				"* * * * * ${LOG_SENDER_SCRIPT} \"${STROOM_BASE_LOGS_DIR}/access\" STROOM-ACCESS-EVENTS \"${STROOM_DATAFEED_URL}\" --system \"${STROOM_SYSTEM_NAME}\" --environment \"${STROOM_ENVIRONMENT_NAME}\" --file-regex \"${STROOM_FILE_REGEX}\" -m ${STROOM_MAX_DELAY_SECS} --delete-after-sending --no-secure --compress > /dev/stdout\n" +
				"* * * * * ${LOG_SENDER_SCRIPT} \"${STROOM_BASE_LOGS_DIR}/app\"    STROOM-APP-EVENTS    \"${STROOM_DATAFEED_URL}\" --system \"{STROOM_SYSTEM_NAME}\" --environment \"${STROOM_ENVIRONMENT_NAME}\" --file-regex \"${STROOM_FILE_REGEX}\" -m ${STROOM_MAX_DELAY_SECS} --delete-after-sending --no-secure --compress > /dev/stdout\n" +
				"* * * * * ${LOG_SENDER_SCRIPT} \"${STROOM_BASE_LOGS_DIR}/user\"   STROOM-USER-EVENTS   \"${STROOM_DATAFEED_URL}\" --system \"{STROOM_SYSTEM_NAME}\" --environment \"${STROOM_ENVIRONMENT_NAME}\" --file-regex \"${STROOM_FILE_REGEX}\" -m ${STROOM_MAX_DELAY_SECS} --delete-after-sending --no-secure --compress > /dev/stdout",
		},
	}

	ctrl.SetControllerReference(stroomCluster, &configMap, r.Scheme)
	return &configMap
}

func (r *StroomClusterReconciler) createCliJob(stroomCluster *stroomv1.StroomCluster, dbInfo *DatabaseConnectionInfo, name string, command []string, args []string) *batchv1.Job {
	volumes := make([]corev1.Volume, 0)
	volumeMounts := make([]corev1.VolumeMount, 0)

	volumes = append(volumes, *r.createStaticContentVolume(stroomCluster))
	r.appendConfigVolumes(stroomCluster, &volumes, &volumeMounts)

	job := batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stroomCluster.GetCliJobName(name),
			Namespace: stroomCluster.Namespace,
			Labels:    stroomCluster.GetLabels(),
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:            "stroom-cli",
						Image:           stroomCluster.Spec.Image.String(),
						ImagePullPolicy: stroomCluster.Spec.ImagePullPolicy,
						Command:         command,
						Args:            args,
						Env: []corev1.EnvVar{{
							Name:  "JAVA_OPTS",
							Value: "-XX:InitialRAMPercentage=50 -XX:MaxRAMPercentage=75",
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
							Name: "STROOM_NODE",
							ValueFrom: &corev1.EnvVarSource{
								FieldRef: &corev1.ObjectFieldSelector{
									FieldPath: "metadata.labels['statefulset.kubernetes.io/pod-name']",
								},
							},
						}},
						VolumeMounts: volumeMounts,
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								"cpu":    *resource.NewMilliQuantity(500, resource.DecimalSI),
								"memory": *resource.NewScaledQuantity(1, resource.Giga),
							},
							Limits: corev1.ResourceList{
								"cpu":    *resource.NewMilliQuantity(2000, resource.DecimalSI),
								"memory": *resource.NewScaledQuantity(2, resource.Giga),
							},
						},
					}},
					Volumes: volumes,
				},
			},
		},
	}

	ctrl.SetControllerReference(stroomCluster, &job, r.Scheme)
	return &job
}

func (r *StroomClusterReconciler) createStaticContentVolume(stroomCluster *stroomv1.StroomCluster) *corev1.Volume {
	return &corev1.Volume{
		Name: "static-content",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: stroomCluster.GetStaticContentConfigMapName(),
				},
			},
		},
	}
}

func (r *StroomClusterReconciler) createStatefulSet(stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet, dbInfo *DatabaseConnectionInfo) *appsv1.StatefulSet {
	secretFileMode := stroomv1.SecretFileMode
	logSender := stroomCluster.Spec.LogSender

	volumes := []corev1.Volume{
		*r.createStaticContentVolume(stroomCluster),
		{
			Name: "api-key",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: stroomCluster.GetBaseName(),
					Items: []corev1.KeyToPath{{
						Key:  "api_key",
						Path: "api_key",
						Mode: &secretFileMode,
					}},
				},
			},
		},
	}
	if !logSender.IsZero() {
		volumes = append(volumes, corev1.Volume{
			Name: "log-sender-configmap",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: stroomCluster.GetLogSenderConfigMapName(),
					},
				},
			},
		})
	}

	volumeMounts := []corev1.VolumeMount{{
		Name:      "static-content",
		SubPath:   "node-start.sh",
		MountPath: "/stroom/scripts/node-start.sh",
		ReadOnly:  true,
	}, {
		Name:      "static-content",
		SubPath:   "pre-stop.sh",
		MountPath: "/stroom/scripts/pre-stop.sh",
		ReadOnly:  true,
	}, {
		Name:      "api-key",
		SubPath:   "api_key",
		MountPath: StroomNodeApiKeyPath,
		ReadOnly:  true,
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
		SubPath:   "lmdb_library",
		MountPath: "/stroom/lmdb_library",
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
		SubPath:   "search-results",
		MountPath: "/stroom/search_results",
	}}

	r.appendConfigVolumes(stroomCluster, &volumes, &volumeMounts)

	if len(stroomCluster.Spec.ExtraVolumes) > 0 {
		volumes = append(volumes, stroomCluster.Spec.ExtraVolumes...)
	}
	if len(stroomCluster.Spec.ExtraVolumeMounts) > 0 {
		volumeMounts = append(volumeMounts, stroomCluster.Spec.ExtraVolumeMounts...)
	}

	containers := []corev1.Container{{
		Name:            StroomNodeContainerName,
		Image:           stroomCluster.Spec.Image.String(),
		ImagePullPolicy: stroomCluster.Spec.ImagePullPolicy,
		Env: append([]corev1.EnvVar{{
			Name:  "API_GATEWAY_HOST",
			Value: stroomCluster.Spec.Ingress.HostName,
		}, {
			Name:  "API_KEY",
			Value: StroomNodeApiKeyPath,
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
			Value: fmt.Sprintf("%v.%v.svc", stroomCluster.GetNodeSetServiceName(nodeSet), stroomCluster.Namespace),
		}, {
			Name:  "JAVA_OPTS",
			Value: r.getJvmOptions(stroomCluster, nodeSet),
		}, {
			Name:  "STROOM_APP_PORT",
			Value: strconv.Itoa(AppPortNumber),
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
		}}, stroomCluster.Spec.ExtraEnv...),
		VolumeMounts:    volumeMounts,
		SecurityContext: &nodeSet.SecurityContext,
		Ports: []corev1.ContainerPort{{
			Name:          AppPortName,
			ContainerPort: AppPortNumber,
			Protocol:      corev1.ProtocolTCP,
		}, {
			Name:          AdminPortName,
			ContainerPort: AdminPortNumber,
			Protocol:      corev1.ProtocolTCP,
		}},
		StartupProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"bash",
						"/stroom/scripts/node-start.sh",
					},
				},
			},
			InitialDelaySeconds: nodeSet.StartupProbeTimings.InitialDelaySeconds,
			PeriodSeconds:       nodeSet.StartupProbeTimings.PeriodSeconds,
			TimeoutSeconds:      nodeSet.StartupProbeTimings.TimeoutSeconds,
			SuccessThreshold:    nodeSet.StartupProbeTimings.SuccessThreshold,
			FailureThreshold:    nodeSet.StartupProbeTimings.FailureThreshold,
		},
		ReadinessProbe: r.createProbe(&nodeSet.ReadinessProbeTimings, AdminPortName),
		LivenessProbe:  r.createProbe(&nodeSet.LivenessProbeTimings, AdminPortName),
		Resources:      nodeSet.Resources,
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"bash",
						"/stroom/scripts/pre-stop.sh",
					},
				},
			},
		},
	}}

	if !logSender.IsZero() {
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

		containers = append(containers, corev1.Container{
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
			VolumeMounts: []corev1.VolumeMount{{
				Name:      StroomNodePvcName,
				SubPath:   "logs",
				MountPath: "/stroom-log-sender/log-volumes/stroom",
			}, {
				Name:      "log-sender-configmap",
				MountPath: "/stroom-log-sender/config",
				ReadOnly:  true,
			}},
			Resources: resources,
		})
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
			ServiceName:         stroomCluster.GetNodeSetServiceName(nodeSet),
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

func (r *StroomClusterReconciler) appendConfigVolumes(stroomCluster *stroomv1.StroomCluster, volumes *[]corev1.Volume, volumeMounts *[]corev1.VolumeMount) {
	// If a Stroom node config override is provided, mount the existing ConfigMap
	configOverride := stroomCluster.Spec.ConfigMapRef
	const configMountPath = "/stroom/config/config.yml"
	if !configOverride.IsZero() {
		*volumes = append(*volumes, corev1.Volume{
			Name: "config-override",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configOverride.Name,
					},
				},
			},
		})
		*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
			Name:      "config-override",
			SubPath:   configOverride.ItemName,
			MountPath: configMountPath,
			ReadOnly:  true,
		})
	} else {
		// Use the default config
		*volumeMounts = append(*volumeMounts, corev1.VolumeMount{
			Name:      "static-content",
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

	var jvmOpts = []string{
		// Support Stroom WebSockets
		"--add-opens java.base/java.nio=ALL-UNNAMED",
		"--add-opens java.base/sun.nio.ch=ALL-UNNAMED",
		"--add-opens java.base/java.lang=ALL-UNNAMED",
	}

	// Set
	jvmOpts = append(jvmOpts, "--add-opens java.base/java.nio=ALL-UNNAMED --add-opens java.base/sun.nio.ch=ALL-UNNAMED --add-opens java.base/java.lang=ALL-UNNAMED")

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

func (r *StroomClusterReconciler) createProbe(probeTimings *stroomv1.ProbeTimings, portName string) *corev1.Probe {
	probe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/stroomAdmin/healthcheck",
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
		probe.PeriodSeconds = probeTimings.InitialDelaySeconds
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
				Name:     AppPortName,
				Port:     AppPortNumber,
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

	// Create an Ingress for each route in each NodeSet, where Ingress is enabled
	for _, nodeSet := range stroomCluster.Spec.NodeSets {
		clusterName := stroomCluster.GetBaseName()
		serviceName := stroomCluster.GetNodeSetServiceName(&nodeSet)

		if nodeSet.IngressEnabled != true {
			continue
		}

		if nodeSet.Role != stroomv1.ProcessingNodeRole {
			ingressAnnotations := map[string]string{
				"nginx.ingress.kubernetes.io/affinity":        "cookie",
				"nginx.ingress.kubernetes.io/affinity-mode":   "persistent",
				"nginx.ingress.kubernetes.io/proxy-body-size": "0", // Disable client request payload size checking
			}

			// Apply any user-provided annotations
			for k, v := range nodeSet.IngressAnnotations {
				ingressAnnotations[k] = v
			}

			ingresses = append(ingresses,
				netv1.Ingress{
					// Default route (/)
					ObjectMeta: metav1.ObjectMeta{
						Name:        clusterName,
						Namespace:   stroomCluster.Namespace,
						Labels:      stroomCluster.GetLabels(),
						Annotations: ingressAnnotations,
					},
					Spec: netv1.IngressSpec{
						IngressClassName: &ingressSettings.ClassName,
						TLS: []netv1.IngressTLS{{
							Hosts:      []string{ingressSettings.HostName},
							SecretName: ingressSettings.SecretName,
						}},
						Rules: []netv1.IngressRule{
							// Explicitly route datafeed traffic to the first non-UI NodeSet
							r.createIngressRule(ingressSettings.HostName, netv1.PathTypeExact, "/stroom/noauth/datafeed", firstNonUiServiceName),

							// All other traffic is routed to the UI NodeSets
							r.createIngressRule(ingressSettings.HostName, netv1.PathTypePrefix, "/", serviceName),
						},
					},
				})

			ingresses = append(ingresses,
				netv1.Ingress{
					// WebSocket endpoint
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName + "-websocket",
						Namespace: stroomCluster.Namespace,
						Labels:    stroomCluster.GetLabels(),
						Annotations: map[string]string{
							"nginx.ingress.kubernetes.io/proxy-http-version": "1.1",
							"nginx.ingress.kubernetes.io/configuration-snippet": "\n" +
								"proxy_set_header Upgrade $http_upgrade;\n" +
								"proxy_set_header Connection \"Upgrade\";\n",
						},
					},
					Spec: netv1.IngressSpec{
						IngressClassName: &ingressSettings.ClassName,
						TLS: []netv1.IngressTLS{{
							Hosts:      []string{ingressSettings.HostName},
							SecretName: ingressSettings.SecretName,
						}},
						Rules: []netv1.IngressRule{
							r.createIngressRule(ingressSettings.HostName, netv1.PathTypePrefix, "/web-socket/", serviceName),
						},
					},
				},
			)
		}

		if nodeSet.Role != stroomv1.FrontendNodeRole {
			ingressAnnotations := map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target":  "/stroom/noauth/datafeed",
				"nginx.ingress.kubernetes.io/proxy-body-size": "0", // Disable client request payload size checking
			}

			// Apply any user-provided annotations
			for k, v := range nodeSet.IngressAnnotations {
				ingressAnnotations[k] = v
			}

			ingresses = append(ingresses, netv1.Ingress{
				// Rewrite requests to `/stroom/datafeeddirect` to `/stroom/noauth/datafeed`
				ObjectMeta: metav1.ObjectMeta{
					Name:        clusterName + "-datafeed",
					Namespace:   stroomCluster.Namespace,
					Labels:      stroomCluster.GetLabels(),
					Annotations: ingressAnnotations,
				},
				Spec: netv1.IngressSpec{
					IngressClassName: &ingressSettings.ClassName,
					TLS: []netv1.IngressTLS{{
						Hosts:      []string{ingressSettings.HostName},
						SecretName: ingressSettings.SecretName,
					}},
					Rules: []netv1.IngressRule{
						r.createIngressRule(ingressSettings.HostName, netv1.PathTypeExact, "/stroom/datafeeddirect", serviceName),
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

func (r *StroomClusterReconciler) createIngressRule(hostName string, pathType netv1.PathType, path string, serviceName string) netv1.IngressRule {
	return netv1.IngressRule{
		Host: hostName,
		IngressRuleValue: netv1.IngressRuleValue{
			HTTP: &netv1.HTTPIngressRuleValue{
				Paths: []netv1.HTTPIngressPath{{
					Path:     path,
					PathType: &pathType,
					Backend: netv1.IngressBackend{
						Service: &netv1.IngressServiceBackend{
							Name: serviceName,
							Port: netv1.ServiceBackendPort{
								Name: AppPortName,
							},
						},
					},
				}},
			},
		},
	}
}
