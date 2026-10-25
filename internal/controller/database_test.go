package controller

import (
	"fmt"
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
		Context("readiness probe", func() {
			// runProbe runs the readiness probe command, against a fake mysql client that exits with the given status
			runProbe := func(mysqlExitStatus int) error {
				if _, err := exec.LookPath("sh"); err != nil {
					Skip("sh is required to run the fake mysql client")
				}
				bin := GinkgoT().TempDir()
				fakeMysql := fmt.Sprintf("#!/bin/sh\nexit %v\n", mysqlExitStatus)
				Expect(os.WriteFile(filepath.Join(bin, "mysql"), []byte(fakeMysql), 0o755)).To(Succeed())

				command := reconciler.createStatefulSet(newDatabaseServer("stroom")).Spec.Template.Spec.Containers[0].ReadinessProbe.Exec.Command
				cmd := exec.Command(filepath.Join(bin, command[0]), command[1:]...)
				return cmd.Run()
			}

			It("should report ready when MySQL can be queried", func() {
				Expect(runProbe(0)).To(Succeed())
			})
			It("should report not ready when MySQL can't be queried", func() {
				Expect(runProbe(1)).ToNot(Succeed())
			})
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

		Context("backup command", func() {
			// runBackup runs the backup command against a fake mysqldump, which records its arguments and exits with
			// the given status, returning the command's output and the backup directory
			runBackup := func(mysqldumpExitStatus int) (string, string, error) {
				for _, tool := range []string{"bash", "gzip"} {
					if _, err := exec.LookPath(tool); err != nil {
						Skip(tool + " is required to run the backup command")
					}
				}
				root := GinkgoT().TempDir()
				bin := filepath.Join(root, "bin")
				Expect(os.MkdirAll(bin, 0o755)).To(Succeed())
				fakeMysqldump := fmt.Sprintf("#!/bin/sh\necho \"$*\" > %v/arguments\necho '-- dump'\nexit %v\n", root, mysqldumpExitStatus)
				Expect(os.WriteFile(filepath.Join(bin, "mysqldump"), []byte(fakeMysqldump), 0o755)).To(Succeed())

				backupRoot := filepath.Join(root, "backup")
				cmd := exec.Command("bash", "-c", newCronJobCommand("stroom"))
				cmd.Env = append(os.Environ(),
					"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
					"BACKUP_ROOT="+backupRoot,
					"MYSQL_USER=stroomuser", "MYSQL_PWD=s3cret", "MYSQL_HOST=db", "MYSQL_PORT=3306",
				)
				output, err := cmd.CombinedOutput()
				arguments, _ := os.ReadFile(filepath.Join(root, "arguments"))
				Expect(string(arguments)).ToNot(ContainSubstring("s3cret"), "the password should not be an argument")
				return string(output), backupRoot, err
			}
			archives := func(backupRoot string) []string {
				files, err := filepath.Glob(filepath.Join(backupRoot, "*", "backup_*.sql.gz"))
				Expect(err).ToNot(HaveOccurred())
				return files
			}

			It("should write a read-only archive of the dump", func() {
				output, backupRoot, err := runBackup(0)
				Expect(err).ToNot(HaveOccurred(), output)
				Expect(output).To(ContainSubstring("Backup successful"))
				files := archives(backupRoot)
				Expect(files).To(HaveLen(1))
				info, err := os.Stat(files[0])
				Expect(err).ToNot(HaveOccurred())
				Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o444)))
			})
			It("should fail before dumping when the image has no gzip", func() {
				if _, err := exec.LookPath("bash"); err != nil {
					Skip("bash is required to run the backup command")
				}
				root := GinkgoT().TempDir()
				bashPath, err := exec.LookPath("bash")
				Expect(err).ToNot(HaveOccurred())
				// Only mysqldump is on the PATH
				Expect(os.WriteFile(filepath.Join(root, "mysqldump"), []byte("#!/bin/sh\necho dumped\n"), 0o755)).To(Succeed())
				cmd := exec.Command(bashPath, "-c", newCronJobCommand("stroom"))
				cmd.Env = []string{"PATH=" + root, "BACKUP_ROOT=" + filepath.Join(root, "backup")}
				output, err := cmd.CombinedOutput()
				Expect(err).To(HaveOccurred(), string(output))
				Expect(string(output)).To(ContainSubstring("Backup failed: the image has no gzip"))
				Expect(string(output)).ToNot(ContainSubstring("dumped"))
			})

			It("should fail, and leave no archive, when the dump fails", func() {
				output, backupRoot, err := runBackup(2)
				Expect(err).To(HaveOccurred(), output)
				Expect(output).To(ContainSubstring("Backup failed"))
				Expect(output).ToNot(ContainSubstring("Backup successful"))
				Expect(archives(backupRoot)).To(BeEmpty())
			})
		})
	})
})
