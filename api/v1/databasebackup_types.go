/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatabaseBackupSpec defines the desired state of DatabaseBackup
type DatabaseBackupSpec struct {
	// +kubebuilder:validation:Required
	Image           Image             `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// DatabaseServerRef contains either the details of a DatabaseServer resource, or the TCP connection details of
	// an external MySQL database
	// +kubebuilder:validation:Required
	DatabaseServerRef DatabaseServerRef `json:"databaseServerRef"`
	// Backup the specified database names. If unspecified, all user databases are backed up
	DatabaseNames []string `json:"databaseNames,omitempty"`
	// File system location to store the backup files
	TargetVolume corev1.VolumeSource `json:"volume"`
	// Cron schedule that determines how often backups are to be performed
	Schedule string `json:"schedule"`
}

// DatabaseBackupStatus defines the observed state of DatabaseBackup
type DatabaseBackupStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DatabaseBackup is the Schema for the databasebackups API
type DatabaseBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseBackupSpec   `json:"spec,omitempty"`
	Status DatabaseBackupStatus `json:"status,omitempty"`
}

// GetBaseName creates a name incorporating the name of the database. For Example: stroom-prod-db
func (in *DatabaseBackup) GetBaseName() string {
	return fmt.Sprintf("stroom-%v-db-backup", in.Name)
}

func (in *DatabaseBackup) GetLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "stroom",
		"app.kubernetes.io/component": "database-backup",
		"app.kubernetes.io/instance":  in.Name,
	}
}

//+kubebuilder:object:root=true

// DatabaseBackupList contains a list of DatabaseBackup
type DatabaseBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseBackup{}, &DatabaseBackupList{})
}
