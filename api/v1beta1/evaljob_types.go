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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Represent a job's status
// +kubebuilder:validation:Enum=Scheduled;Running;Complete;Cancelled
type JobState string

const (
	// The job is scheduled and waiting for available resources to run it
	ScheduledJobState JobState = "Scheduled"
	// The job is running
	RunningJobState JobState = "Running"
	// The job is complete
	CompleteJobState JobState = "Complete"
	// The job is cancelled
	CancelledJobState JobState = "Cancelled"
)

// +kubebuilder:validation:Enum=NoResult;Succeeded;Failed;Cancelled
type Result string

const (
	// Job is still running and no final result yet
	NoResult Result = "NoResult"
	// Job finished successfully
	SucceedResult Result = "Succeeded"
	// Job failed
	FailedResult Result = "Failed"
	// Job is cancelled
	CancelledResult Result = "Cancelled"
)

type Arg struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

// EvalJobSpec defines the desired state of EvalJob
type EvalJobSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Model name
	Model string `json:"model"`
	// Args for the model
	// +optional
	ModelArgs []Arg `json:"modelArgs,omitempty"`
	// Evaluation tasks
	Tasks []string `json:"tasks"`
	// Sets the number of few-shot examples to place in context
	// +optional
	NumFewShot *int `json:"numFewShot,omitempty"`
	// Accepts an integer, or a float between 0.0 and 1.0 . If passed, will limit
	// the number of documents to evaluate to the first X documents (if an integer)
	// per task or first X% of documents per task
	// +optional
	Limit string `json:"limit,omitempty"`
	// Map to `--gen_kwargs` parameter for the underlying library.
	// +optional
	GenArgs []Arg `json:"genArgs,omitempty"`
	// If this flag is passed, then the model's outputs, and the text fed into the
	// model, will be saved at per-document granularity
	// +optional
	LogSamples *bool `json:"logSamples,omitempty"`
}

// EvalJobStatus defines the observed state of EvalJob
type EvalJobStatus struct {
	// Important: Run "make" to regenerate code after modifying this file

	// The name of the Pod that runs the evaluation job
	// +optional
	PodName string `json:"podName,omitempty"`
	// Status of the job
	// +optional
	State JobState `json:"state,omitempty"`
	// Final result of the job
	// +optional
	Result Result `json:"result,omitempty"`
	// Message about the current/final status
	// +optional
	Message string `json:"message,omitempty"`
	// Information when was the last time the job was successfully scheduled.
	// +optional
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// EvalJob is the Schema for the evaljobs API
type EvalJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EvalJobSpec   `json:"spec,omitempty"`
	Status EvalJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EvalJobList contains a list of EvalJob
type EvalJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EvalJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EvalJob{}, &EvalJobList{})
}
