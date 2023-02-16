/*
Copyright 2023.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CronDeletePVCSpec defines the desired state of CronDeletePVC
type CronDeletePVCSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	//+kubebuilder:validation:MinLength=0
	// The schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
	Schedule string `json:"schedule"`

	//This flag tells the controller to suspend subsequent execution, Default to false
	// +optional
	Suspend *bool `json:"suspend,omitempty"`

	//+kubebuilder:default=10080
	//+kubebuilder:validation:Minimum=60
	// ReserveMinutes delete pvc before reserveMinutes, minimum 60, default 10080
	ReserveMinutes *int32 `json:"reserveMinutes,omitempty"`

	//+kubebuilder:default=100
	//+kubebuilder:validation:Minimum=20
	//+kubebuilder:validation:Maximum=1000
	// DeletedPVCHistoryLimit  the number of deleted pvc record in status,minimum 20,max 1000 default 100
	DeletedPVCHistoryLimit *int32 `json:"deletedPVCHistoryLimit,omitempty"`

	//+kubebuilder:default=tidb-operator
	//ManagedBy record managed by label,default tidb-operator
	ManagedBy string `json:"ManagedBy"`
}

// CronDeletePVCStatus defines the observed state of CronDeletePVC
type CronDeletePVCStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Information when was the last time the job was successfully scheduled.
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

	// WaitingDeletePVCs record waiting delete pvcs
	WaitingDeletePVCs map[string]PVC `json:"waitingDeletePVCs,omitempty"`
	// DeletedPVCs record deleted pvcs, store for history limit count
	DeletedPVCs []PVC `json:"DeletedPVCs,omitempty"`
}

// PVC record pvc info unused by pod
type PVC struct {
	Name        string       `json:"name"`
	Namespace   string       `json:"namespace"`
	UID         types.UID    `json:"uid"`
	UnUseTime   *metav1.Time `json:"unuseTime"`
	DeletedTime *metav1.Time `json:"deleteTime,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// CronDeletePVC is the Schema for the crondeletepvcs API
type CronDeletePVC struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CronDeletePVCSpec   `json:"spec,omitempty"`
	Status CronDeletePVCStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CronDeletePVCList contains a list of CronDeletePVC
type CronDeletePVCList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CronDeletePVC `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CronDeletePVC{}, &CronDeletePVCList{})
}
