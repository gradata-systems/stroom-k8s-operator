package databaseserver

import (
	"fmt"
	stroomv1 "github.com/p-kimberley/stroom-k8s-operator/api/v1"
	"github.com/p-kimberley/stroom-k8s-operator/controllers/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"strings"
)

func GetBaseName(componentName string) string {
	return fmt.Sprintf("stroom-%v-db", componentName)
}

func (r *DatabaseServerReconciler) getServiceName(dbServer *stroomv1.DatabaseServer) string {
	return fmt.Sprintf("%v-headless", GetBaseName(dbServer.Name))
}

func (r *DatabaseServerReconciler) getInitConfigName(dbServer *stroomv1.DatabaseServer) string {
	return fmt.Sprintf("%v-init", GetBaseName(dbServer.Name))
}

var (
	// DefaultSecretFileMode is the file mode to use for Secret volume mounts
	DefaultSecretFileMode int32 = 0400

	ServiceUserName       = "stroomuser"
	DatabasePort    int32 = 3306
)

func (r *DatabaseServerReconciler) createLabels(dbServer *stroomv1.DatabaseServer) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "stroom",
		"app.kubernetes.io/component": "database-server",
		"app.kubernetes.io/instance":  dbServer.Name,
	}
}

func (r *DatabaseServerReconciler) createSecret(dbServer *stroomv1.DatabaseServer) *corev1.Secret {
	labels := r.createLabels(dbServer)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetBaseName(dbServer.Name),
			Namespace: dbServer.Namespace,
			Labels:    labels,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"root":          common.GeneratePassword(),
			ServiceUserName: common.GeneratePassword(),
		},
	}

	ctrl.SetControllerReference(dbServer, secret, r.Scheme)
	return secret
}

func (r *DatabaseServerReconciler) createConfigMap(dbServer *stroomv1.DatabaseServer) *corev1.ConfigMap {
	labels := r.createLabels(dbServer)

	additionalConfig := ""
	if dbServer.Spec.AdditionalConfig != nil {
		additionalConfig = strings.Join(dbServer.Spec.AdditionalConfig, "\n")
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetBaseName(dbServer.Name),
			Namespace: dbServer.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"my.cnf": "" +
				"[mysqld]\n" +
				"datadir=/var/lib/mysql\n" +
				"port=" + string(DatabasePort) + "\n" +
				"user=mysql\n" +
				additionalConfig,
		},
	}

	ctrl.SetControllerReference(dbServer, configMap, r.Scheme)
	return configMap
}

func (r *DatabaseServerReconciler) createDbInitConfigMap(dbServer *stroomv1.DatabaseServer) *corev1.ConfigMap {
	labels := r.createLabels(dbServer)

	databaseCreateStatements := ""
	for databaseName := range dbServer.Spec.DatabaseNames {
		databaseCreateStatements += "" +
			fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %v;\n", databaseName) +
			fmt.Sprintf("GRANT ALL PRIVILEGES ON %v.* TO '%v'@'%%';\n", databaseName, ServiceUserName)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getInitConfigName(dbServer),
			Namespace: dbServer.Namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			"create-service-user.sql": "" +
				"-- Create a service user for determining MySQL health\n" +
				"CREATE USER 'healthcheck'@'localhost';",
			"init-stroom-db.sql": "" +
				"-- Initialise Stroom databases\n" +
				databaseCreateStatements + "\n\n" +
				"SELECT 'Dumping list of databases' AS '';\n" +
				"SELECT '---------------------------------------' AS '';\n" +
				"SHOW databases;",
		},
	}

	ctrl.SetControllerReference(dbServer, configMap, r.Scheme)
	return configMap
}

func (r *DatabaseServerReconciler) createStatefulSet(dbServer *stroomv1.DatabaseServer) *appsv1.StatefulSet {
	labels := r.createLabels(dbServer)
	var replicas int32 = 1

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetBaseName(dbServer.Name),
			Namespace: dbServer.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: r.getServiceName(dbServer),
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: dbServer.Annotations,
					Labels:      labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:            "mysql",
						Image:           dbServer.Spec.Image,
						ImagePullPolicy: dbServer.Spec.ImagePullPolicy,
						Env: []corev1.EnvVar{{
							Name:  "MYSQL_ROOT_PASSWORD",
							Value: "/etc/mysql/password/root",
						}, {
							Name:  "MYSQL_USER",
							Value: ServiceUserName,
						}, {
							Name: "MYSQL_PASSWORD",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: GetBaseName(dbServer.Name),
									},
									Key: ServiceUserName,
								},
							},
						}},
						Ports: []corev1.ContainerPort{{
							Name:          "mysql",
							ContainerPort: DatabasePort,
							Protocol:      corev1.ProtocolTCP,
						}},
						ReadinessProbe:  &dbServer.Spec.ReadinessProbe,
						LivenessProbe:   &dbServer.Spec.LivenessProbe,
						SecurityContext: &dbServer.Spec.SecurityContext,
						Resources:       dbServer.Spec.Resources,
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "config",
							MountPath: "/etc/my.cnf",
							SubPath:   "my.cnf",
							ReadOnly:  true,
						}, {
							Name:      "config-init",
							MountPath: "/docker-entrypoint-initdb.d",
							ReadOnly:  true,
						}, {
							Name:      "data",
							MountPath: "/var/lib/mysql",
						}, {
							Name:      "root-password",
							MountPath: "/etc/mysql/password/root",
							SubPath:   "root",
							ReadOnly:  true,
						}},
					}},
					SecurityContext: &dbServer.Spec.PodSecurityContext,
					NodeSelector:    dbServer.Spec.NodeSelector,
					Affinity:        &dbServer.Spec.Affinity,
					Tolerations:     dbServer.Spec.Tolerations,
					Volumes: []corev1.Volume{{
						Name: "config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: GetBaseName(dbServer.Name),
								},
							},
						},
					}, {
						Name: "config-init",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: r.getInitConfigName(dbServer),
								},
							},
						},
					}, {
						Name: "root-password",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: GetBaseName(dbServer.Name),
								Items: []corev1.KeyToPath{{
									Key:  "root",
									Path: "root",
									Mode: &DefaultSecretFileMode,
								}},
							},
						},
					}},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "data",
				},
				Spec: dbServer.Spec.VolumeClaim,
			}},
		},
	}

	ctrl.SetControllerReference(dbServer, statefulSet, r.Scheme)
	return statefulSet
}
