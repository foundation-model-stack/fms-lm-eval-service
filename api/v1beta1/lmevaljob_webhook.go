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
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var evaljoblog = logf.Log.WithName("evaljob-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *LMEvalJob) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-foundation-model-stack-github-com-github-com-v1beta1-lmevaljob,mutating=true,failurePolicy=fail,sideEffects=None,groups=foundation-model-stack.github.com.github.com,resources=lmevaljobs,verbs=create;update,versions=v1beta1,name=mlmevaljob.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &LMEvalJob{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *LMEvalJob) Default() {
	evaljoblog.Info("default", "name", r.Name)
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-foundation-model-stack-github-com-github-com-v1beta1-lmevaljob,mutating=false,failurePolicy=fail,sideEffects=None,groups=foundation-model-stack.github.com.github.com,resources=lmevaljobs,verbs=create;update,versions=v1beta1,name=vlmevaljob.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &LMEvalJob{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *LMEvalJob) ValidateCreate() (admission.Warnings, error) {
	evaljoblog.Info("validate create", "name", r.Name)

	return nil, r.ValidateJob()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *LMEvalJob) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	evaljoblog.Info("validate update", "name", r.Name)

	return nil, r.ValidateJob()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *LMEvalJob) ValidateDelete() (admission.Warnings, error) {
	evaljoblog.Info("validate delete", "name", r.Name)

	return nil, nil
}

func (r *LMEvalJob) ValidateJob() error {
	var allErrs field.ErrorList
	if err := r.ValidateLimit(); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: GroupName, Kind: KindName}, r.Name, allErrs)
}

func (r *LMEvalJob) ValidateLimit() *field.Error {
	if r.Spec.Limit == "" {
		return nil
	}

	_, notInterr := strconv.ParseInt(r.Spec.Limit, 10, 64)
	if notInterr == nil {
		return nil
	}
	if limit, notFloatErr := strconv.ParseFloat(r.Spec.Limit, 64); notFloatErr == nil && limit <= 1.0 && limit > 0.0 {

		return nil
	}
	return field.Invalid(field.NewPath("spec").Child("limit"), r.Spec.Limit, "must  be an integer or a float number between 0.0 to 1.0")
}
