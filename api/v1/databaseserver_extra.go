package v1

import corev1 "k8s.io/api/core/v1"

type BackupSettings struct {
	DatabaseNames []string            `json:"databaseNames,omitempty"`
	TargetVolume  corev1.VolumeSource `json:"volume"`
	Schedule      string              `json:"schedule"`
}

func (in *BackupSettings) IsUnset() bool {
	return len(in.DatabaseNames) == 0 && in.TargetVolume == (corev1.VolumeSource{}) && in.Schedule == ""
}
