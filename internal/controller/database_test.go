package controller

import (
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
)

var _ = Describe("Database resources", func() {

	Context("DatabaseServer", func() {
		reconciler := &DatabaseServerReconciler{Scheme: scheme.Scheme}
		newDatabaseServer := func(databaseNames ...string) *stroomv1.DatabaseServer {
			return &stroomv1.DatabaseServer{
				ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "default"},
				Spec:       stroomv1.DatabaseServerSpec{DatabaseNames: databaseNames},
			}
		}

		It("should create each database and grant the service user access to it", func() {
			sql := reconciler.createDbInitConfigMap(newDatabaseServer("stroom", "stats")).Data["init-stroom-db.sql"]
			Expect(sql).To(ContainSubstring("CREATE DATABASE IF NOT EXISTS `stroom`;\nGRANT ALL PRIVILEGES ON `stroom`.* TO 'stroomuser'@'%';"))
			Expect(sql).To(ContainSubstring("CREATE DATABASE IF NOT EXISTS `stats`;\nGRANT ALL PRIVILEGES ON `stats`.* TO 'stroomuser'@'%';"))
		})
		It("should quote database names in the init SQL", func() {
			// The CRD only accepts plain names, so this guards against names that bypass validation
			sql := reconciler.createDbInitConfigMap(newDatabaseServer("x`; DROP DATABASE mysql; -- ")).Data["init-stroom-db.sql"]
			Expect(sql).To(ContainSubstring("CREATE DATABASE IF NOT EXISTS `x``; DROP DATABASE mysql; -- `;"))
		})
		It("should report ready even when MySQL can't be queried", func() {
			// BASELINE: known bug. The readiness probe prints its result instead of exiting with it, so the pod is
			// ready before MySQL accepts connections.
			if _, err := exec.LookPath("sh"); err != nil {
				Skip("sh is required to run the readiness probe")
			}
			bin := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(bin, "mysql"), []byte("#!/bin/sh\nexit 1\n"), 0o755)).To(Succeed())

			command := reconciler.createStatefulSet(newDatabaseServer("stroom")).Spec.Template.Spec.Containers[0].ReadinessProbe.Exec.Command
			cmd := exec.Command(command[0], command[1:]...)
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			Expect(cmd.Run()).To(Succeed())
		})
		It("should set the root password from the generated Secret", func() {
			container := reconciler.createStatefulSet(newDatabaseServer("stroom")).Spec.Template.Spec.Containers[0]
			Expect(container.Env).To(ContainElement(corev1.EnvVar{Name: "MYSQL_ROOT_PASSWORD", ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "stroom-db-db"},
					Key:                  "root",
				},
			}}))
			Expect(container.VolumeMounts).ToNot(ContainElement(HaveField("MountPath", "/etc/mysql/password/root")))
		})
	})

	Context("DatabaseBackup", func() {
		reconciler := &DatabaseBackupReconciler{Scheme: scheme.Scheme}
		newCronJobCommand := func(databaseNames ...string) string {
			dbBackup := &stroomv1.DatabaseBackup{
				ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "default"},
				Spec: stroomv1.DatabaseBackupSpec{
					DatabaseNames: databaseNames,
					TargetVolume:  corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				},
			}
			cronJob := reconciler.createCronJob(dbBackup, goldenDatabaseConnectionInfo("db"))
			return cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Command[2]
		}

		It("should back up all databases when none are listed", func() {
			Expect(newCronJobCommand()).To(ContainSubstring("--all-databases"))
		})
		It("should back up the listed databases", func() {
			Expect(newCronJobCommand("stroom", "stats")).To(ContainSubstring("--databases 'stroom' 'stats'"))
		})
		It("should quote database names in the backup shell command", func() {
			// The CRD only accepts plain names, so this guards against names that bypass validation
			Expect(newCronJobCommand("$(id)")).To(ContainSubstring(`--databases '$(id)'`))
			Expect(newCronJobCommand("$(id)")).To(ContainSubstring(`echo "Backing up "'databases ($(id))'" to: $archivePath"`))
		})
	})
})
