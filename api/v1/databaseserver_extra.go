package v1

import corev1 "k8s.io/api/core/v1"

type BackupSettings struct {
	// Backup the specified database names. If unspecified, all user databases are backed up
	DatabaseNames []string `json:"databaseNames,omitempty"`
	// File system location to store the backup files
	TargetVolume corev1.VolumeSource `json:"volume,omitempty"`
	// Cron schedule that determines how often backups are to be performed
	Schedule string `json:"schedule,omitempty"`
}

func (in *BackupSettings) IsZero() bool {
	return len(in.DatabaseNames) == 0 && in.TargetVolume == (corev1.VolumeSource{}) && in.Schedule == ""
}
