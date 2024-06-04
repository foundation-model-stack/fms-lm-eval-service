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

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	lmevalservicev1beta1 "github.com/foundation-model-stack/fms-lm-eval-service/api/v1beta1"
	"github.com/go-logr/logr"
)

// EvalJobReconciler reconciles a EvalJob object
type EvalJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=lm-eval-service.github.com,resources=evaljobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=lm-eval-service.github.com,resources=evaljobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=lm-eval-service.github.com,resources=evaljobs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;watch

func (r *EvalJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	evalJob := &lmevalservicev1beta1.EvalJob{}
	if err := r.Get(ctx, req.NamespacedName, evalJob); err != nil {
		log.Error(err, "unable to fetch EvalJob. could be from an deletion request")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !evalJob.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion here
		return r.handleFinalizer(ctx, evalJob, log)
	}

	// Handle the job that is just created
	if evalJob.Status.LastScheduleTime == nil {
		return r.handleNewCR(ctx, log, &req.NamespacedName, evalJob)
	}

	// Handle the update events of the job
	switch evalJob.Status.State {
	case lmevalservicev1beta1.ScheduledJobState:
		pod, err := r.getPod(ctx, evalJob)
		if err != nil {
			// a weird state, someone delete the corresponding pod? mark this as CompleteJobState
			// with error message
			evalJob.Status.State = lmevalservicev1beta1.CompleteJobState
			evalJob.Status.Result = lmevalservicev1beta1.FailedResult
			evalJob.Status.Message = err.Error()
			if err := r.Status().Update(ctx, evalJob); err != nil {
				log.Error(err, "unable to update EvalJob status", "state", evalJob.Status.State)
				r.Get(ctx, req.NamespacedName, evalJob)
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		if pod.Status.ContainerStatuses == nil {
			return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
		}
		for _, cstatus := range pod.Status.ContainerStatuses {
			if cstatus.Name == "main" {
				if cstatus.LastTerminationState.Terminated == nil {
					return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
				} else {
					if cstatus.LastTerminationState.Terminated.ExitCode == 0 {
						evalJob.Status.State = lmevalservicev1beta1.CompleteJobState
						evalJob.Status.Result = lmevalservicev1beta1.SucceedResult
					} else {
						evalJob.Status.State = lmevalservicev1beta1.CompleteJobState
						evalJob.Status.Result = lmevalservicev1beta1.FailedResult
						evalJob.Status.Message = cstatus.LastTerminationState.Terminated.Reason

					}
					if err := r.Status().Update(ctx, evalJob); err != nil {
						log.Error(err, "unable to update EvalJob status", "state", evalJob.Status.State)
						r.Get(ctx, req.NamespacedName, evalJob)
						return ctrl.Result{}, err
					}
					return ctrl.Result{}, nil
				}
			}
		}
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
	case lmevalservicev1beta1.RunningJobState:
		log.Info("JobState is Running. Waiting for next state", "state", evalJob.Status.State)
	case lmevalservicev1beta1.CompleteJobState:
		log.Info("JobState is Complete. Job is done", "state", evalJob.Status.State)
	case lmevalservicev1beta1.CancelledJobState:
		log.Info("JobState is Cancelled. Job is done", "state", evalJob.Status.State)
	}

	return ctrl.Result{}, nil
}

func (r *EvalJobReconciler) handleFinalizer(ctx context.Context, evalJob *lmevalservicev1beta1.EvalJob, log logr.Logger) (reconcile.Result, error) {
	if controllerutil.ContainsFinalizer(evalJob, lmevalservicev1beta1.FinalizerName) {
		// delete the correspondling pod
		// remove our finalizer from the list and update it.
		if err := r.deleteJobPod(ctx, evalJob); err != nil {
			log.Error(err, "failed to delete pod of the job")
		}

		controllerutil.RemoveFinalizer(evalJob, lmevalservicev1beta1.FinalizerName)
		if err := r.Update(ctx, evalJob); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Successfully remove the finalizer", "name", evalJob.Name)
	}

	return ctrl.Result{}, nil
}

func (r *EvalJobReconciler) handleNewCR(ctx context.Context, log logr.Logger, namespacedName *types.NamespacedName, job *lmevalservicev1beta1.EvalJob) (reconcile.Result, error) {
	// If it doesn't contain our finalizer, add it
	if !controllerutil.ContainsFinalizer(job, lmevalservicev1beta1.FinalizerName) {
		controllerutil.AddFinalizer(job, lmevalservicev1beta1.FinalizerName)
		if err := r.Update(ctx, job); err != nil {
			log.Error(err, "unable to update finalizer")
			return ctrl.Result{}, err
		}
		// Since finalizers were updated. Need to fetch the new EvalJob
		// End the current reconsile and get revisioned job in next reconsile
		return ctrl.Result{}, nil
	}

	// construct a new pod and create a pod for the job
	currentTime := v1.Now()
	pod := createPod(job)
	if err := r.Create(ctx, pod, &client.CreateOptions{}); err != nil {
		// Failed to create the pod. Mark the status as complete with failed
		job.Status.State = lmevalservicev1beta1.CompleteJobState
		job.Status.Result = lmevalservicev1beta1.FailedResult
		job.Status.Message = err.Error()
		if err := r.Status().Update(ctx, job); err != nil {
			log.Error(err, "unable to update EvalJob status for pod creation failure")
		}
		log.Error(err, "Failed to create pod for the EvalJob", "name", *namespacedName)
		r.Get(ctx, *namespacedName, job)
		return ctrl.Result{}, err
	}

	// Create the pod successfully. Wait for the driver to update the status
	job.Status.State = lmevalservicev1beta1.ScheduledJobState
	job.Status.PodName = pod.Name
	job.Status.LastScheduleTime = &currentTime
	if err := r.Status().Update(ctx, job); err != nil {
		log.Error(err, "unable to update EvalJob status (pod creation done)")
		r.Get(ctx, *namespacedName, job)
		return ctrl.Result{}, err
	}

	log.Info("Successfully create a Pod for the Job")
	// Check the pod after 10 seconds
	return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 10}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *EvalJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&lmevalservicev1beta1.EvalJob{}).
		Complete(r)
}

func (r *EvalJobReconciler) getPod(ctx context.Context, job *lmevalservicev1beta1.EvalJob) (*corev1.Pod, error) {
	var pod = corev1.Pod{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: job.Namespace, Name: job.Name}, &pod); err != nil {
		return nil, err
	}
	for _, ref := range pod.OwnerReferences {
		if ref.APIVersion == job.APIVersion &&
			ref.Kind == job.Kind &&
			ref.Name == job.Name {

			return &pod, nil
		}
	}
	return nil, fmt.Errorf("pod doesn't have proper entry in the OwnerReferences")
}

func (r *EvalJobReconciler) deleteJobPod(ctx context.Context, job *lmevalservicev1beta1.EvalJob) error {
	pod := corev1.Pod{
		TypeMeta: v1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      job.Status.PodName,
			Namespace: job.Namespace,
			OwnerReferences: []v1.OwnerReference{
				{
					APIVersion: job.APIVersion,
					Kind:       job.Kind,
					Name:       job.Name,
				},
			},
		},
	}
	return r.Delete(ctx, &pod, &client.DeleteOptions{})
}

func createPod(job *lmevalservicev1beta1.EvalJob) *corev1.Pod {
	var allowPrivilegeEscalation = false
	var runAsNonRootUser = true
	var ownerRefController = true
	var runAsUser int64 = 1001030000

	// Prepare the Command of the main container from EvalJob
	command := generateCmd(job)

	// Then compose the Pod CR
	pod := corev1.Pod{
		TypeMeta: v1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      job.Name,
			Namespace: job.Namespace,
			OwnerReferences: []v1.OwnerReference{
				{
					APIVersion: job.APIVersion,
					Kind:       job.Kind,
					Name:       job.Name,
					Controller: &ownerRefController,
					UID:        job.UID,
				},
			},
			Labels: map[string]string{
				"app.kubernetes.io/name": "fms-lm-eval-service",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:            "main",
					Image:           "quay.io/yhwang/lm-eval-aas-flask:test",
					ImagePullPolicy: corev1.PullAlways,
					Env: []corev1.EnvVar{
						{
							Name: "GENAI_KEY",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									Key: "key",
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "genai-key",
									},
								},
							},
						},
					},
					Command: command,
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: &allowPrivilegeEscalation,
						RunAsUser:                &runAsUser,
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{
								"ALL",
							},
						},
					},
				},
			},
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: &runAsNonRootUser,
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
		},
	}
	return &pod
}

func generateCmd(job *lmevalservicev1beta1.EvalJob) []string {
	if job == nil {
		return nil
	}

	cmds := make([]string, 0, 10)
	cmds = append(cmds, "python", "-m", "lm-eval")
	// --model
	cmds = append(cmds, "--model", job.Spec.Model)
	// --model_args
	if job.Spec.ModelArgs != nil {
		cmds = append(cmds, "--model_args", argsToString(job.Spec.ModelArgs))
	}
	// --tasks
	cmds = append(cmds, "--tasks", strings.Join(job.Spec.Tasks, ","))
	// --num_fewshot
	if job.Spec.NumFewShot != nil {
		cmds = append(cmds, "--num_fewshot", fmt.Sprintf("%d", *job.Spec.NumFewShot))
	}
	// --limit
	if job.Spec.Limit != "" {
		cmds = append(cmds, "--limit", job.Spec.Limit)
	}
	// --gen_kwargs
	if job.Spec.GenArgs != nil {
		cmds = append(cmds, "--gen_kwargs", argsToString(job.Spec.GenArgs))
	}
	// --log_samples
	if job.Spec.LogSamples != nil && *job.Spec.LogSamples {
		cmds = append(cmds, "--log_samples")
	}

	return cmds
}

func argsToString(args []lmevalservicev1beta1.Arg) string {
	if args == nil {
		return ""
	}
	var equalForms []string
	for _, arg := range args {
		equalForms = append(equalForms, fmt.Sprintf("%s=%s", arg.Name, arg.Value))
	}
	return strings.Join(equalForms, ",")
}
