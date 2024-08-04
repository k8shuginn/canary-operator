/*
Copyright 2024.

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
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CanarySpec defines the desired state of Canary
type CanarySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// OldDeployment defines the old deployment to transition from
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	OldDeployment string `json:"oldDeployment"`

	// NewDeployment defines the new deployment to transition to
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	NewDeployment string `json:"newDeployment"`

	// TotalReplicas defines the total number of replicas to scale up/down
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	TotalReplicas int32 `json:"totalReplicas"`

	// StepReplicas defines the number of replicas to scale up/down in each step
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	StepReplicas int32 `json:"stepReplicas"`

	// CronSchedule defines the cron schedule to run the canary
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	CronSchedule string `json:"cronSchedule"`

	// EnableRollback defines whether to enable rollback or not
	// +operator-sdk:csv:customresourcedefinitions:type=spec
	EnableRollback bool `json:"enableRollback"`
}

// CanaryStatus defines the observed state of Canary
type CanaryStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// OldReplicas defines the old number of replicas
	// +operator-sdk:csv:customresourcedefinitions:type=status
	OldReplicas int32 `json:"oldReplicas"`

	// NewReplicas defines the new number of replicas
	// +operator-sdk:csv:customresourcedefinitions:type=status
	NewReplicas int32 `json:"newReplicas"`

	// CurrentStep defines the current step count
	// +operator-sdk:csv:customresourcedefinitions:type=status
	CurrentStep int32 `json:"currentStep"`

	// State defines the current state of the canary
	// +operator-sdk:csv:customresourcedefinitions:type=status
	State string `json:"state"`

	// Message defines the state message of the canary
	// +operator-sdk:csv:customresourcedefinitions:type=status
	Message string `json:"message"`
}

//+kubebuilder:printcolumn:name="OldReplicas",type="integer",JSONPath=".status.oldReplicas"
//+kubebuilder:printcolumn:name="NewReplicas",type="integer",JSONPath=".status.newReplicas"
//+kubebuilder:printcolumn:name="CurrentStep",type="integer",JSONPath=".status.currentStep"
//+kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state"
//+kubebuilder:printcolumn:name="Message",type="string",JSONPath=".status.message",priority=1
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Canary is the Schema for the canaries API
type Canary struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CanarySpec   `json:"spec,omitempty"`
	Status CanaryStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CanaryList contains a list of Canary
type CanaryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Canary `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Canary{}, &CanaryList{})
}
