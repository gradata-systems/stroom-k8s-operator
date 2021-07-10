package controllers

import (
	"context"
	_ "embed"
	"fmt"
	stroomv1 "github.com/p-kimberley/stroom-k8s-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"math"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
	"strings"
)

const (
	AppPortName             = "app"
	AppPortNumber           = 8080
	AdminPortName           = "admin"
	AdminPortNumber         = 8081
	StroomNodeApiKeyPath    = "/stroom/auth/api_key"
	StroomNodePvcName       = "data"
	StroomNodeContainerName = "stroom-node"
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
			Name:      stroomCluster.GetBaseName(),
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
				"* * * * * ${LOG_SENDER_SCRIPT} ${STROOM_BASE_LOGS_DIR}/access STROOM-ACCESS-EVENTS ${DATAFEED_URL} --system STROOM --environment ${DEFAULT_ENVIRONMENT} --file-regex \"${DEFAULT_FILE_REGEX}\" -m ${MAX_DELAY_SECS} --delete-after-sending --no-secure --compress > /dev/stdout\n" +
				"* * * * * ${LOG_SENDER_SCRIPT} ${STROOM_BASE_LOGS_DIR}/app    STROOM-APP-EVENTS    ${DATAFEED_URL} --system STROOM --environment ${DEFAULT_ENVIRONMENT} --file-regex \"${DEFAULT_FILE_REGEX}\" -m ${MAX_DELAY_SECS} --delete-after-sending --no-secure --compress > /dev/stdout\n" +
				"* * * * * ${LOG_SENDER_SCRIPT} ${STROOM_BASE_LOGS_DIR}/user   STROOM-USER-EVENTS   ${DATAFEED_URL} --system STROOM --environment ${DEFAULT_ENVIRONMENT} --file-regex \"${DEFAULT_FILE_REGEX}\" -m ${MAX_DELAY_SECS} --delete-after-sending --no-secure --compress > /dev/stdout",
		},
	}

	ctrl.SetControllerReference(stroomCluster, &configMap, r.Scheme)
	return &configMap
}

func (r *StroomClusterReconciler) createStatefulSet(stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet, dbInfo *DatabaseConnectionInfo) *appsv1.StatefulSet {
	secretFileMode := stroomv1.SecretFileMode
	logSender := stroomCluster.Spec.LogSender

	volumes := []corev1.Volume{{
		Name: "static-content",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: stroomCluster.GetBaseName(),
				},
			},
		},
	}, {
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
	}}
	if logSender.Enabled {
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
		SubPath:   "stroomcluster-config.yaml",
		MountPath: "/stroom/config/config.yml",
		ReadOnly:  true,
	}, {
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

	// Shared volumes are optional and for UI nodes, it makes sense to omit them
	if nodeSet.SharedDataVolume != (corev1.VolumeSource{}) {
		volumes = append(volumes, corev1.Volume{
			Name:         "stroom-shared",
			VolumeSource: nodeSet.SharedDataVolume,
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "stroom-shared",
			MountPath: "/stroom/volumes",
		})
	}

	if len(stroomCluster.Spec.ExtraVolumes) > 0 {
		volumes = append(volumes, stroomCluster.Spec.ExtraVolumes...)
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
			Value: DatabaseServiceUserName,
		}, {
			Name: "STROOM_JDBC_DRIVER_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: dbInfo.SecretName,
					},
					Key: DatabaseServiceUserName,
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
			Value: DatabaseServiceUserName,
		}, {
			Name: "STROOM_STATISTICS_JDBC_DRIVER_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: dbInfo.SecretName,
					},
					Key: DatabaseServiceUserName,
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
			Handler: corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"bash",
						"/stroom/scripts/node-start.sh",
					},
				},
			},
			InitialDelaySeconds: 10,
			PeriodSeconds:       5,
			TimeoutSeconds:      3,
			SuccessThreshold:    1,
			FailureThreshold:    10,
		},
		ReadinessProbe: r.createProbe(&nodeSet.ReadinessProbeTimings, AdminPortName),
		LivenessProbe:  r.createProbe(&nodeSet.LivenessProbeTimings, AdminPortName),
		Resources:      nodeSet.Resources,
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.Handler{
				Exec: &corev1.ExecAction{
					Command: []string{
						"bash",
						"/stroom/scripts/pre-stop.sh",
					},
				},
			},
		},
	}}

	if logSender.Enabled {
		destinationUrl := logSender.DestinationUrl
		if destinationUrl == "" {
			destinationUrl = stroomCluster.GetDatafeedUrl()
		}

		environmentName := logSender.EnvironmentName
		if environmentName == "" {
			environmentName = strings.ToUpper(stroomCluster.Name)
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
				Name:  "DATAFEED_URL",
				Value: destinationUrl,
			}, {
				Name:  "STROOM_BASE_LOGS_DIR",
				Value: "/stroom-log-sender/log-volumes/stroom",
			}, {
				Name:  "DEFAULT_FILE_REGEX",
				Value: `.*/[a-z]+-[0-9]+-[0-9]+-[0-9]+T[0-9]+:[0-9]+\.log(\.gz)?$`,
			}, {
				Name:  "DEFAULT_ENVIRONMENT",
				Value: environmentName,
			}, {
				Name:  "MAX_DELAY_SECS",
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
		})
	}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stroomCluster.GetNodeSetName(nodeSet),
			Namespace: stroomCluster.Namespace,
			Labels:    stroomCluster.GetLabels(),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &nodeSet.Count,
			ServiceName: stroomCluster.GetNodeSetServiceName(nodeSet),
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

	// Calculate the size of the Java heap
	const javaHeapLimitMB int64 = 30 * 1024
	maxMemory := nodeSet.Resources.Limits.Memory().ScaledValue(resource.Mega) / 2

	memory := int64(math.Floor(math.Min(float64(maxMemory), float64(javaHeapLimitMB))))
	return fmt.Sprintf("-Xms%vm -Xmx%vm", memory, memory)
}

func (r *StroomClusterReconciler) createProbe(probeTimings *stroomv1.ProbeTimings, portName string) *corev1.Probe {
	probe := &corev1.Probe{
		Handler: corev1.Handler{
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

func (r *StroomClusterReconciler) createService(stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stroomCluster.GetNodeSetServiceName(nodeSet),
			Namespace: stroomCluster.Namespace,
			Labels:    stroomCluster.GetLabels(),
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
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

func (r *StroomClusterReconciler) createIngresses(ctx context.Context, stroomCluster *stroomv1.StroomCluster) []v1.Ingress {
	logger := log.FromContext(ctx)
	ingressSettings := stroomCluster.Spec.Ingress
	var ingresses []v1.Ingress

	// Find out the first non-UI NodeSet so we know where to route datafeed traffic to
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
			ingresses = append(ingresses,
				v1.Ingress{
					// Default route (/)
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: stroomCluster.Namespace,
						Labels:    stroomCluster.GetLabels(),
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":                 "nginx",
							"nginx.ingress.kubernetes.io/affinity":        "cookie",
							"nginx.ingress.kubernetes.io/proxy-body-size": "0", // Disable client request payload size checking
						},
					},
					Spec: v1.IngressSpec{
						TLS: []v1.IngressTLS{{
							Hosts:      []string{ingressSettings.HostName},
							SecretName: ingressSettings.SecretName,
						}},
						Rules: []v1.IngressRule{
							// Explicitly route datafeed traffic to the first non-UI NodeSet
							r.createIngressRule(ingressSettings.HostName, v1.PathTypeExact, "/stroom/noauth/datafeed", firstNonUiServiceName),

							// All other traffic is routed to the UI NodeSets
							r.createIngressRule(ingressSettings.HostName, v1.PathTypePrefix, "/", serviceName),
						},
					},
				},
				v1.Ingress{
					// Deny access to the `/stroom/clustercall.rpc` endpoint
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName + "-clustercall",
						Namespace: stroomCluster.Namespace,
						Labels:    stroomCluster.GetLabels(),
						Annotations: map[string]string{
							"kubernetes.io/ingress.class":                "nginx",
							"nginx.ingress.kubernetes.io/server-snippet": "location ~ .*/clustercall.rpc$ { deny all; }",
						},
					},
					Spec: v1.IngressSpec{
						TLS: []v1.IngressTLS{{
							Hosts:      []string{ingressSettings.HostName},
							SecretName: ingressSettings.SecretName,
						}},
						Rules: []v1.IngressRule{
							r.createIngressRule(ingressSettings.HostName, v1.PathTypeExact, "/clustercall.rpc", serviceName),
						},
					},
				})
		}

		if nodeSet.Role != stroomv1.FrontendNodeRole {
			ingresses = append(ingresses, v1.Ingress{
				// Rewrite requests to `/stroom/datafeeddirect` to `/stroom/noauth/datafeed`
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName + "-datafeed",
					Namespace: stroomCluster.Namespace,
					Labels:    stroomCluster.GetLabels(),
					Annotations: map[string]string{
						"kubernetes.io/ingress.class":                "nginx",
						"nginx.ingress.kubernetes.io/rewrite-target": "/stroom/noauth/datafeed",
					},
				},
				Spec: v1.IngressSpec{
					TLS: []v1.IngressTLS{{
						Hosts:      []string{ingressSettings.HostName},
						SecretName: ingressSettings.SecretName,
					}},
					Rules: []v1.IngressRule{
						r.createIngressRule(ingressSettings.HostName, v1.PathTypeExact, "/stroom/datafeeddirect", serviceName),
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

func (r *StroomClusterReconciler) createIngressRule(hostName string, pathType v1.PathType, path string, serviceName string) v1.IngressRule {
	return v1.IngressRule{
		Host: hostName,
		IngressRuleValue: v1.IngressRuleValue{
			HTTP: &v1.HTTPIngressRuleValue{
				Paths: []v1.HTTPIngressPath{{
					Path:     path,
					PathType: &pathType,
					Backend: v1.IngressBackend{
						Service: &v1.IngressServiceBackend{
							Name: serviceName,
							Port: v1.ServiceBackendPort{
								Name: AppPortName,
							},
						},
					},
				}},
			},
		},
	}
}
