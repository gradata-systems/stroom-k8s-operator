package controllers

import (
	"fmt"
	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"strconv"
	"strings"
)

const (
	DatabaseRootUserName          = "root"
	DatabaseServiceUserName       = "stroomuser"
	DatabasePort            int32 = 3306
)

func (r *DatabaseServerReconciler) getInitConfigName(dbServer *stroomv1.DatabaseServer) string {
	return fmt.Sprintf("%v-init", dbServer.GetBaseName())
}

func (r *DatabaseServerReconciler) createSecret(dbServer *stroomv1.DatabaseServer) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbServer.GetSecretName(),
			Namespace: dbServer.Namespace,
			Labels:    dbServer.GetLabels(),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			DatabaseRootUserName:    stroomv1.GeneratePassword(),
			DatabaseServiceUserName: stroomv1.GeneratePassword(),
		},
	}

	// Do not set the controller reference, as we want the Secret to persist if the DatabaseServer is deleted

	return secret
}

func (r *DatabaseServerReconciler) createConfigMap(dbServer *stroomv1.DatabaseServer) *corev1.ConfigMap {
	additionalConfig := ""
	if dbServer.Spec.AdditionalConfig != nil {
		additionalConfig = strings.Join(dbServer.Spec.AdditionalConfig, "\n")
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbServer.GetBaseName(),
			Namespace: dbServer.Namespace,
			Labels:    dbServer.GetLabels(),
		},
		Data: map[string]string{
			"my.cnf": "" +
				"[mysqld]\n" +
				"datadir=/var/lib/mysql\n" +
				"port=" + strconv.Itoa(int(DatabasePort)) + "\n" +
				"user=mysql\n" +
				additionalConfig,
		},
	}

	ctrl.SetControllerReference(dbServer, configMap, r.Scheme)
	return configMap
}

func (r *DatabaseServerReconciler) createDbInitConfigMap(dbServer *stroomv1.DatabaseServer) *corev1.ConfigMap {
	databaseCreateStatements := ""
	for _, databaseName := range dbServer.Spec.DatabaseNames {
		databaseCreateStatements += "" +
			fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %v;\n", databaseName) +
			fmt.Sprintf("GRANT ALL PRIVILEGES ON %v.* TO '%v'@'%%';\n", databaseName, DatabaseServiceUserName)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.getInitConfigName(dbServer),
			Namespace: dbServer.Namespace,
			Labels:    dbServer.GetLabels(),
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
	var replicas int32 = 1

	// DefaultSecretFileMode is the file mode to use for Secret volume mounts
	secretFileMode := stroomv1.SecretFileMode

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbServer.GetBaseName(),
			Namespace: dbServer.Namespace,
			Labels:    dbServer.GetLabels(),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: dbServer.GetServiceName(),
			Selector: &metav1.LabelSelector{
				MatchLabels: dbServer.GetLabels(),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: dbServer.Annotations,
					Labels:      dbServer.GetLabels(),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:            "mysql",
						Image:           dbServer.Spec.Image.String(),
						ImagePullPolicy: dbServer.Spec.ImagePullPolicy,
						Env: []corev1.EnvVar{{
							Name:  "MYSQL_ROOT_PASSWORD",
							Value: "/etc/mysql/password/root",
						}, {
							Name:  "MYSQL_USER",
							Value: DatabaseServiceUserName,
						}, {
							Name: "MYSQL_PASSWORD",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: dbServer.GetBaseName(),
									},
									Key: DatabaseServiceUserName,
								},
							},
						}},
						Ports: []corev1.ContainerPort{{
							Name:          "mysql",
							ContainerPort: DatabasePort,
							Protocol:      corev1.ProtocolTCP,
						}},
						ReadinessProbe:  r.createReadinessProbe(dbServer.Spec.ReadinessProbeTimings),
						LivenessProbe:   r.createLivenessProbe(dbServer.Spec.LivenessProbeTimings),
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
							SubPath:   DatabaseRootUserName,
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
									Name: dbServer.GetBaseName(),
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
								SecretName: dbServer.GetBaseName(),
								Items: []corev1.KeyToPath{{
									Key:  DatabaseRootUserName,
									Path: DatabaseRootUserName,
									Mode: &secretFileMode,
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

func (r *DatabaseServerReconciler) createReadinessProbe(timings stroomv1.ProbeTimings) *corev1.Probe {
	return &corev1.Probe{
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"sh",
					"-c",
					"status=$(mysql -u healthcheck -e \"SELECT 'OK'\" | grep 'OK');\n" +
						"if [[ $status ]]; then echo 1; else echo 0; fi",
				},
			},
		},
		InitialDelaySeconds: timings.InitialDelaySeconds,
		PeriodSeconds:       timings.PeriodSeconds,
		TimeoutSeconds:      timings.TimeoutSeconds,
		SuccessThreshold:    timings.SuccessThreshold,
		FailureThreshold:    timings.FailureThreshold,
	}
}

func (r *DatabaseServerReconciler) createLivenessProbe(timings stroomv1.ProbeTimings) *corev1.Probe {
	return &corev1.Probe{
		Handler: corev1.Handler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"mysqladmin", "-u", "healthcheck", "ping",
				},
			},
		},
		InitialDelaySeconds: timings.InitialDelaySeconds,
		PeriodSeconds:       timings.PeriodSeconds,
		TimeoutSeconds:      timings.TimeoutSeconds,
		SuccessThreshold:    timings.SuccessThreshold,
		FailureThreshold:    timings.FailureThreshold,
	}
}

func (r *DatabaseServerReconciler) createService(dbServer *stroomv1.DatabaseServer) *corev1.Service {
	labels := dbServer.GetLabels()

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbServer.GetServiceName(),
			Namespace: dbServer.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
			Selector:  labels,
			Ports: []corev1.ServicePort{{
				Name:     "tcp",
				Port:     DatabasePort,
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}

	ctrl.SetControllerReference(dbServer, service, r.Scheme)
	return service
}
