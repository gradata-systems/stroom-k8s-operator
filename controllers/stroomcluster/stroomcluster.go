package stroomcluster

import (
	"context"
	"fmt"
	stroomv1 "github.com/p-kimberley/stroom-k8s-operator/api/v1"
	"github.com/p-kimberley/stroom-k8s-operator/controllers/databaseserver"
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
)

const (
	AppPortName     = "app"
	AppPortNumber   = 8080
	AdminPortName   = "admin"
	AdminPortNumber = 8081
)

func GetBaseName(clusterName string) string {
	return fmt.Sprintf("stroom-%v", clusterName)
}

func GetStroomNodeSetName(clusterName string, nodeSetName string) string {
	return fmt.Sprintf("stroom-%v-node-%v", clusterName, nodeSetName)
}

func GetStroomNodeSetServiceName(clusterName string, nodeSetName string) string {
	return fmt.Sprintf("%v-http", GetStroomNodeSetName(clusterName, nodeSetName))
}

func (r *StroomClusterReconciler) createLabels(stroomCluster *stroomv1.StroomCluster) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "stroom",
		"app.kubernetes.io/component": "stroom-cluster",
		"stroom/cluster":              stroomCluster.Name,
	}
}

func (r *StroomClusterReconciler) createNodeSetSelectorLabels(stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet) map[string]string {
	return map[string]string{
		"stroom/cluster":     stroomCluster.Name,
		"stroom/nodeSet":     nodeSet.Name,
		"stroom/nodeSetRole": string(nodeSet.Role),
	}
}

func (r *StroomClusterReconciler) createServiceAccount(stroomCluster *stroomv1.StroomCluster) *corev1.ServiceAccount {
	serviceAccount := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetBaseName(stroomCluster.Name),
			Namespace: stroomCluster.Namespace,
			Labels:    r.createLabels(stroomCluster),
		},
	}

	ctrl.SetControllerReference(stroomCluster, &serviceAccount, r.Scheme)
	return &serviceAccount
}

func (r *StroomClusterReconciler) createStatefulSet(stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet,
	appDatabase *databaseserver.DatabaseConnectionInfo, statsDatabase *databaseserver.DatabaseConnectionInfo) *appsv1.StatefulSet {
	selectorLabels := r.createNodeSetSelectorLabels(stroomCluster, nodeSet)

	volumes := []corev1.Volume{{
		Name: "config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: stroomCluster.Spec.ConfigMapName,
				},
			},
		},
	}}

	volumeMounts := []corev1.VolumeMount{{
		Name:      "config",
		SubPath:   "config.yml",
		MountPath: "/stroom/config/config.yml",
		ReadOnly:  true,
	}, {
		Name:      "data",
		SubPath:   "logs",
		MountPath: "/stroom/logs",
	}, {
		Name:      "data",
		SubPath:   "output",
		MountPath: "/stroom/output",
	}, {
		Name:      "data",
		SubPath:   "tmp",
		MountPath: "/stroom/tmp",
	}, {
		Name:      "data",
		SubPath:   "proxy-repo",
		MountPath: "/stroom/proxy_repo",
	}, {
		Name:      "data",
		SubPath:   "reference-data",
		MountPath: "/stroom/reference_data",
	}, {
		Name:      "data",
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

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetStroomNodeSetName(stroomCluster.Name, nodeSet.Name),
			Namespace: stroomCluster.Namespace,
			Labels:    r.createLabels(stroomCluster),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &nodeSet.Count,
			ServiceName: GetStroomNodeSetServiceName(stroomCluster.Name, nodeSet.Name),
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nodeSet.PodAnnotations,
					Labels:      selectorLabels,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: GetBaseName(stroomCluster.Name),
					SecurityContext:    &nodeSet.PodSecurityContext,
					Containers: []corev1.Container{{
						Name:            "stroom-node",
						Image:           stroomCluster.Spec.Image.String(),
						ImagePullPolicy: stroomCluster.Spec.ImagePullPolicy,
						Env: []corev1.EnvVar{{
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
							Value: fmt.Sprintf("%v.%v.svc", GetStroomNodeSetServiceName(stroomCluster.Name, nodeSet.Name), stroomCluster.Namespace),
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
							Name:  "STROOM_JDBC_DRIVER_URL",
							Value: appDatabase.ToConnectionString(),
						}, {
							Name:  "STROOM_JDBC_DRIVER_CLASS_NAME",
							Value: "com.mysql.cj.jdbc.Driver",
						}, {
							Name:  "STROOM_JDBC_DRIVER_USERNAME",
							Value: databaseserver.ServiceUserName,
						}, {
							Name: "STROOM_JDBC_DRIVER_PASSWORD",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: appDatabase.SecretName,
									},
									Key: databaseserver.ServiceUserName,
								},
							},
						}, {
							Name:  "STROOM_STATISTICS_JDBC_DRIVER_URL",
							Value: statsDatabase.ToConnectionString(),
						}, {
							Name:  "STROOM_STATISTICS_JDBC_DRIVER_CLASS_NAME",
							Value: "com.mysql.cj.jdbc.Driver",
						}, {
							Name:  "STROOM_STATISTICS_JDBC_DRIVER_USERNAME",
							Value: databaseserver.ServiceUserName,
						}, {
							Name: "STROOM_STATISTICS_JDBC_DRIVER_PASSWORD",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: statsDatabase.SecretName,
									},
									Key: databaseserver.ServiceUserName,
								},
							},
						}},
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
						StartupProbe:  r.createProbe(&nodeSet.StartupProbeTimings, AdminPortName),
						LivenessProbe: r.createProbe(&nodeSet.LivenessProbeTimings, AdminPortName),
						Resources:     nodeSet.Resources,
					}},
					Volumes:      volumes,
					NodeSelector: nodeSet.NodeSelector,
					Affinity:     &nodeSet.Affinity,
					Tolerations:  nodeSet.Tolerations,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "data",
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
			Name:      GetStroomNodeSetServiceName(stroomCluster.Name, nodeSet.Name),
			Namespace: stroomCluster.Namespace,
			Labels:    r.createLabels(stroomCluster),
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
			Selector:  r.createNodeSetSelectorLabels(stroomCluster, nodeSet),
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
	labels := r.createLabels(stroomCluster)
	ingressSettings := stroomCluster.Spec.Ingress
	var ingresses []v1.Ingress

	// Find out the first non-UI NodeSet so we know where to route datafeed traffic to
	firstNonUiServiceName := ""
	for _, nodeSet := range stroomCluster.Spec.NodeSets {
		if nodeSet.Role != stroomv1.Frontend {
			firstNonUiServiceName = GetStroomNodeSetServiceName(stroomCluster.Name, nodeSet.Name)
			break
		}
	}

	// Create an Ingress for each route in each NodeSet, where Ingress is enabled
	for _, nodeSet := range stroomCluster.Spec.NodeSets {
		clusterName := GetBaseName(stroomCluster.Name)
		serviceName := GetStroomNodeSetServiceName(stroomCluster.Name, nodeSet.Name)

		if nodeSet.IngressEnabled != true {
			continue
		}

		if nodeSet.Role != stroomv1.Processing {
			ingresses = append(ingresses,
				v1.Ingress{
					// Default route (/)
					ObjectMeta: metav1.ObjectMeta{
						Name:      clusterName,
						Namespace: stroomCluster.Namespace,
						Labels:    labels,
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
						Labels:    labels,
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

		if nodeSet.Role != stroomv1.Frontend {
			ingresses = append(ingresses, v1.Ingress{
				// Rewrite requests to `/stroom/datafeeddirect` to `/stroom/noauth/datafeed`
				ObjectMeta: metav1.ObjectMeta{
					Name:      clusterName + "-datafeed",
					Namespace: stroomCluster.Namespace,
					Labels:    labels,
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
