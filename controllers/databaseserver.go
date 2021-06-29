package controllers

import (
	"fmt"
	stroomv1 "github.com/p-kimberley/stroom-k8s-operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/batch/v1"
	"k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path"
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

func (r *DatabaseServerReconciler) createLabels(dbName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "stroom",
		"app.kubernetes.io/component": "database-server",
		"app.kubernetes.io/instance":  dbName,
	}
}

func (r *DatabaseServerReconciler) createSecret(dbServer *stroomv1.DatabaseServer) *corev1.Secret {
	labels := r.createLabels(dbServer.Name)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbServer.GetSecretName(),
			Namespace: dbServer.Namespace,
			Labels:    labels,
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
	labels := r.createLabels(dbServer.Name)

	additionalConfig := ""
	if dbServer.Spec.AdditionalConfig != nil {
		additionalConfig = strings.Join(dbServer.Spec.AdditionalConfig, "\n")
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbServer.GetBaseName(),
			Namespace: dbServer.Namespace,
			Labels:    labels,
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
	labels := r.createLabels(dbServer.Name)

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
	labels := r.createLabels(dbServer.Name)
	var replicas int32 = 1

	// DefaultSecretFileMode is the file mode to use for Secret volume mounts
	var secretFileMode int32 = 0400

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbServer.GetBaseName(),
			Namespace: dbServer.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: dbServer.GetServiceName(),
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
					"mysqladmin -u healthcheck ping",
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
	labels := r.createLabels(dbServer.Name)

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

func (r *DatabaseServerReconciler) createCronJob(dbServer *stroomv1.DatabaseServer) *v1beta1.CronJob {
	labels := r.createLabels(dbServer.Name)
	backupSettings := dbServer.Spec.Backup
	const targetDirectory = "/var/lib/mysql/backup"
	const datePattern = "%Y-%m-%d_%H-%M-%S"
	archiveFileName := fmt.Sprintf("$(date +'%v_%v.sql.gz')", dbServer.Name, datePattern)
	backupDirectory := path.Join(targetDirectory, "$(date +'%Y-%m')") // Subdirectory in the format YYYY-MM
	archivePath := path.Join(backupDirectory, archiveFileName)

	mysqlDumpCommand := "mysqldump -u${MYSQL_USER} -p${MYSQL_PASSWORD} -h${MYSQL_HOST} --single-transaction --no-tablespaces"
	plainTextDatabaseList := ""
	if len(backupSettings.DatabaseNames) > 0 {
		mysqlDumpCommand += " --databases " + strings.Join(backupSettings.DatabaseNames, " ")
		plainTextDatabaseList = fmt.Sprintf("databases (%v)", strings.Join(backupSettings.DatabaseNames, ","))
	} else {
		mysqlDumpCommand += " --all-databases"
		plainTextDatabaseList = "all databases"
	}

	cronJob := &v1beta1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbServer.GetBaseName(),
			Namespace: dbServer.Namespace,
			Labels:    labels,
		},
		Spec: v1beta1.CronJobSpec{
			Schedule:          backupSettings.Schedule,
			ConcurrencyPolicy: v1beta1.ForbidConcurrent,
			JobTemplate: v1beta1.JobTemplateSpec{
				Spec: v1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{{
								Name:            "backup-job",
								Image:           dbServer.Spec.Image.String(),
								ImagePullPolicy: corev1.PullIfNotPresent,
								Command: []string{
									"bash",
									"-c",
									fmt.Sprintf("archivePath=\"%v\"", archivePath) + "\n" +
										fmt.Sprintf("echo \"Backing up %v to: $archivePath\"", plainTextDatabaseList) + "\n" +
										fmt.Sprintf("mkdir -p %v", backupDirectory) + "\n" +
										fmt.Sprintf("%v | gzip > \"$archivePath\"", mysqlDumpCommand) + "\n" +
										"if [ -f \"$archivePath\" ]; then\n" +
										"  chmod 444 \"$archivePath\"\n" +
										"  echo \"Backup successful\"\n" +
										"fi",
								},
								Env: []corev1.EnvVar{{
									Name:  "MYSQL_HOST",
									Value: dbServer.GetServiceName(),
								}, {
									Name:  "MYSQL_USER",
									Value: DatabaseServiceUserName,
								}, {
									Name: "MYSQL_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: dbServer.GetSecretName(),
											},
											Key: DatabaseServiceUserName,
										},
									},
								}},
								VolumeMounts: []corev1.VolumeMount{{
									Name:      "data",
									MountPath: "/var/lib/mysql/backup",
								}},
							}},
							Volumes: []corev1.Volume{{
								Name:         "data",
								VolumeSource: backupSettings.TargetVolume,
							}},
						},
					},
				},
			},
		},
	}

	ctrl.SetControllerReference(dbServer, cronJob, r.Scheme)
	return cronJob
}
