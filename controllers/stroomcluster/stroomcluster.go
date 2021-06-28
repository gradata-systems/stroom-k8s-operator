package stroomcluster

import (
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
	return fmt.Sprintf("stroom-%v-%v", clusterName, nodeSetName)
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
						Image:           stroomCluster.Spec.Image,
						ImagePullPolicy: stroomCluster.Spec.ImagePullPolicy,
						Env: []corev1.EnvVar{{
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
						VolumeMounts: []corev1.VolumeMount{{
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
						}, {
							Name:      "stroom-shared",
							MountPath: "/stroom/volumes",
						}},
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
					Volumes: []corev1.Volume{{
						Name: "config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: stroomCluster.Spec.ConfigMapName,
								},
							},
						},
					}, {
						Name:         "stroom-shared",
						VolumeSource: nodeSet.SharedDataVolume,
					}},
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
	return &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/stroomAdmin/healthcheck",
				Port: intstr.IntOrString{
					Type:   intstr.String,
					StrVal: portName,
				},
			},
		},
		InitialDelaySeconds: probeTimings.InitialDelaySeconds,
		TimeoutSeconds:      probeTimings.TimeoutSeconds,
		PeriodSeconds:       probeTimings.PeriodSeconds,
		SuccessThreshold:    probeTimings.SuccessThreshold,
		FailureThreshold:    probeTimings.FailureThreshold,
	}
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

func (r *StroomClusterReconciler) createIngresses(stroomCluster *stroomv1.StroomCluster, serviceName string) []v1.Ingress {
	ingressSettings := &stroomCluster.Spec.Ingress
	hostName := ingressSettings.HostName
	labels := r.createLabels(stroomCluster)

	ingresses := []v1.Ingress{{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetBaseName(stroomCluster.Name),
			Namespace: stroomCluster.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/affinity":        "cookie",
				"nginx.ingress.kubernetes.io/proxy-body-size": "0", // Disable client request payload size checking
			},
		},
		Spec: v1.IngressSpec{
			IngressClassName: &ingressSettings.ClassName,
			TLS: []v1.IngressTLS{{
				Hosts:      []string{hostName},
				SecretName: ingressSettings.SecretName,
			}},
			Rules: r.createIngressRules(hostName, v1.PathTypePrefix, "/", serviceName),
		},
	}, {
		// Rewrite requests to `/stroom/datafeeddirect` to `/stroom/noauth/datafeed`
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetBaseName(stroomCluster.Name) + "-datafeed",
			Namespace: stroomCluster.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/stroom/noauth/datafeed$1$2",
			},
		},
		Spec: v1.IngressSpec{
			IngressClassName: &ingressSettings.ClassName,
			TLS: []v1.IngressTLS{{
				Hosts:      []string{hostName},
				SecretName: ingressSettings.SecretName,
			}},
			Rules: r.createIngressRules(hostName, v1.PathTypePrefix, "/stroom/datafeeddirect(/|$)(.*)", serviceName),
		},
	}, {
		// Deny access to the `/stroom/clustercall.rpc` endpoint
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetBaseName(stroomCluster.Name) + "-clustercall",
			Namespace: stroomCluster.Namespace,
			Labels:    labels,
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/server-snippet": "location ~ .*/clustercall.rpc$ { deny all; }",
			},
		},
		Spec: v1.IngressSpec{
			IngressClassName: &ingressSettings.ClassName,
			TLS: []v1.IngressTLS{{
				Hosts:      []string{hostName},
				SecretName: ingressSettings.SecretName,
			}},
			Rules: r.createIngressRules(hostName, v1.PathTypeExact, "/clustercall.rpc", serviceName),
		},
	}}

	for _, ingress := range ingresses {
		ctrl.SetControllerReference(stroomCluster, &ingress, r.Scheme)
	}

	return ingresses
}

func (r *StroomClusterReconciler) createIngressRules(hostName string, pathType v1.PathType, path string, serviceName string) []v1.IngressRule {
	return []v1.IngressRule{{
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
	}}
}