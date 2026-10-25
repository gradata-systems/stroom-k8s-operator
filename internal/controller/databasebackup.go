package controller

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *DatabaseBackupReconciler) createCronJob(dbBackup *stroomv1.DatabaseBackup, dbInfo *DatabaseConnectionInfo) *batchv1.CronJob {
	const targetDirectory = "/var/lib/mysql/backup"
	const fileDatePattern = "%Y-%m-%d_%H-%M-%S"

	archiveFileName := fmt.Sprintf("$(date +'%v_%v.sql.gz')", dbBackup.Name, fileDatePattern)
	// Subdirectory in the format YYYY-MM. BACKUP_ROOT is only set by tests.
	backupDirectory := path.Join("${BACKUP_ROOT:-"+targetDirectory+"}", "$(date +'%Y-%m')")
	archivePath := path.Join(backupDirectory, archiveFileName)

	// The password is in MYSQL_PWD, which mysqldump reads, as arguments are visible to other processes
	mysqlDumpCommand := "mysqldump --user=${MYSQL_USER} --host=${MYSQL_HOST} --port=${MYSQL_PORT} --single-transaction --no-tablespaces"
	plainTextDatabaseList := ""
	databaseNames := dbBackup.Spec.DatabaseNames
	if len(databaseNames) > 0 {
		quotedDatabaseNames := make([]string, len(databaseNames))
		for i, databaseName := range databaseNames {
			quotedDatabaseNames[i] = shellQuote(databaseName)
		}
		mysqlDumpCommand += " --databases " + strings.Join(quotedDatabaseNames, " ")
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
						// Labelled so a NetworkPolicy can allow backups to reach the database server
						ObjectMeta: metav1.ObjectMeta{
							Labels: dbBackup.GetLabels(),
						},
						Spec: corev1.PodSpec{
							RestartPolicy:                corev1.RestartPolicyOnFailure,
							AutomountServiceAccountToken: &automountServiceAccountToken,
							SecurityContext:              podSecurityContext(corev1.PodSecurityContext{}),
							Containers: []corev1.Container{{
								Name:            "backup-job",
								Image:           dbBackup.Spec.Image.String(),
								ImagePullPolicy: dbBackup.Spec.ImagePullPolicy,
								SecurityContext: containerSecurityContext(corev1.SecurityContext{}),
								Command: []string{
									"bash",
									"-c",
									// Without pipefail, a failed dump would leave an empty archive and report success
									"set -o pipefail\n" +
										// Images differ in what they include, e.g. mysql/mysql-server has no gzip
										"for tool in mysqldump gzip; do\n" +
										"  if ! command -v \"$tool\" > /dev/null; then\n" +
										"    echo \"Backup failed: the image has no $tool\" >&2\n" +
										"    exit 1\n" +
										"  fi\n" +
										"done\n" +
										fmt.Sprintf("backupDirectory=\"%v\"", backupDirectory) + "\n" +
										fmt.Sprintf("archivePath=\"%v\"", archivePath) + "\n" +
										fmt.Sprintf("echo \"Backing up \"%v\" to: $archivePath\"", shellQuote(plainTextDatabaseList)) + "\n" +
										"mkdir -p \"$backupDirectory\" || exit 1\n" +
										fmt.Sprintf("if ! %v | gzip > \"$archivePath\"; then\n", mysqlDumpCommand) +
										"  rm -f \"$archivePath\"\n" +
										"  echo \"Backup failed\" >&2\n" +
										"  exit 1\n" +
										"fi\n" +
										"chmod 444 \"$archivePath\"\n" +
										"echo \"Backup successful\"",
								},
								Env: []corev1.EnvVar{{
									Name:  "MYSQL_HOST",
									Value: dbInfo.Host,
								}, {
									Name:  "MYSQL_PORT",
									Value: strconv.Itoa(int(dbInfo.Port)),
								}, {
									Name:  "MYSQL_USER",
									Value: dbInfo.UserName,
								}, {
									Name: "MYSQL_PWD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: dbInfo.SecretName,
											},
											Key: dbInfo.UserName,
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
