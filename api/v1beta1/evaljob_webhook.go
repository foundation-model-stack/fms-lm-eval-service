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
func (r *EvalJob) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-lm-eval-service-github-com-v1beta1-evaljob,mutating=true,failurePolicy=fail,sideEffects=None,groups=lm-eval-service.github.com,resources=evaljobs,verbs=create;update,versions=v1beta1,name=mevaljob.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &EvalJob{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *EvalJob) Default() {
	evaljoblog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-lm-eval-service-github-com-v1beta1-evaljob,mutating=false,failurePolicy=fail,sideEffects=None,groups=lm-eval-service.github.com,resources=evaljobs,verbs=create;update,versions=v1beta1,name=vevaljob.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &EvalJob{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *EvalJob) ValidateCreate() (admission.Warnings, error) {
	evaljoblog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *EvalJob) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	evaljoblog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *EvalJob) ValidateDelete() (admission.Warnings, error) {
	evaljoblog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

func (r *EvalJob) ValidateJob() error {
	var allErrs field.ErrorList
	if err := r.ValidateLimit(); err != nil {
		allErrs = append(allErrs, err)
	}
	return apierrors.NewInvalid(
		schema.GroupKind{Group: GroupName, Kind: KindName}, r.Name, allErrs)
}

func (r *EvalJob) ValidateLimit() *field.Error {
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
