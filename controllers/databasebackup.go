package controllers

import (
	"fmt"
	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"path"
	ctrl "sigs.k8s.io/controller-runtime"
	"strconv"
	"strings"
)

func (r *DatabaseBackupReconciler) createCronJob(dbBackup *stroomv1.DatabaseBackup, dbInfo *DatabaseConnectionInfo) *batchv1.CronJob {
	const targetDirectory = "/var/lib/mysql/backup"
	const fileDatePattern = "%Y-%m-%d_%H-%M-%S"

	archiveFileName := fmt.Sprintf("$(date +'%v_%v.sql.gz')", dbBackup.Name, fileDatePattern)
	backupDirectory := path.Join(targetDirectory, "$(date +'%Y-%m')") // Subdirectory in the format YYYY-MM
	archivePath := path.Join(backupDirectory, archiveFileName)

	mysqlDumpCommand := "mysqldump --user=${MYSQL_USER} --password=${MYSQL_PASSWORD} --host=${MYSQL_HOST} --port=${MYSQL_PORT} --single-transaction --no-tablespaces"
	plainTextDatabaseList := ""
	databaseNames := dbBackup.Spec.DatabaseNames
	if len(databaseNames) > 0 {
		mysqlDumpCommand += " --databases " + strings.Join(databaseNames, " ")
		plainTextDatabaseList = fmt.Sprintf("databases (%v)", strings.Join(databaseNames, ","))
	} else {
		mysqlDumpCommand += " --all-databases"
		plainTextDatabaseList = "all databases"
	}

	// Retain the CronJob for 5 minutes after it completes
	var ttlSecondsAfterFinished int32 = 300

	cronJob := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbBackup.GetBaseName(),
			Namespace: dbBackup.Namespace,
			Labels:    dbBackup.GetLabels(),
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          dbBackup.Spec.Schedule,
			ConcurrencyPolicy: batchv1.ForbidConcurrent,
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					TTLSecondsAfterFinished: &ttlSecondsAfterFinished,
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyOnFailure,
							Containers: []corev1.Container{{
								Name:            "backup-job",
								Image:           dbBackup.Spec.Image.String(),
								ImagePullPolicy: dbBackup.Spec.ImagePullPolicy,
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
									Value: dbInfo.Host,
								}, {
									Name:  "MYSQL_PORT",
									Value: strconv.Itoa(int(dbInfo.Port)),
								}, {
									Name:  "MYSQL_USER",
									Value: DatabaseServiceUserName,
								}, {
									Name: "MYSQL_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: dbInfo.SecretName,
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
								VolumeSource: dbBackup.Spec.TargetVolume,
							}},
						},
					},
				},
			},
		},
	}

	ctrl.SetControllerReference(dbBackup, cronJob, r.Scheme)
	return cronJob
}
