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
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	lmevalservicev1beta1 "github.com/foundation-model-stack/fms-lm-eval-service/api/v1beta1"
	"github.com/go-logr/logr"
)

const (
	DriverPath                  = "/bin/driver"
	DestDriverPath              = "/opt/app-root/src/bin/driver"
	PodImageKey                 = "pod-image"
	DriverImageKey              = "driver-image"
	DriverServiceAccountKey     = "driver-serviceaccount"
	PodCheckingIntervalKey      = "pod-checking-interval"
	ImagePullPolicyKey          = "image-pull-policy"
	DefaultPodImage             = "quay.io/yhwang/lm-eval-aas-flask:test"
	DefaultDriverImage          = "quay.io/yhwang/lm-eval-aas-driver:test"
	DefaultDriverServiceAccount = "driver"
	DefaultPodCheckingInterval  = time.Second * 10
	DefaultImagePullPolicy      = corev1.PullAlways
)

var (
	pullPolicyMap = map[corev1.PullPolicy]corev1.PullPolicy{
		corev1.PullAlways:       corev1.PullAlways,
		corev1.PullNever:        corev1.PullNever,
		corev1.PullIfNotPresent: corev1.PullIfNotPresent,
	}
)

// LMEvalJobReconciler reconciles a LMEvalJob object
type LMEvalJobReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Recorder  record.EventRecorder
	ConfigMap string
	Namespace string
	options   ServiceOptions
}

type ServiceOptions struct {
	PodImage             string
	DriverImage          string
	DriverServiceAccount string
	PodCheckingInterval  time.Duration
	ImagePullPolicy      corev1.PullPolicy
}

// +kubebuilder:rbac:groups=foundation-model-stack.github.com.github.com,resources=lmevaljobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=foundation-model-stack.github.com.github.com,resources=lmevaljobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=foundation-model-stack.github.com.github.com,resources=lmevaljobs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;watch;list

func (r *LMEvalJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	job := &lmevalservicev1beta1.LMEvalJob{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		log.Info("unable to fetch LMEvalJob. could be from a deletion request")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !job.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion here
		return r.handleDeletion(ctx, job, log)
	}

	// Treat this as NewJobState
	if job.Status.LastScheduleTime == nil {
		job.Status.State = lmevalservicev1beta1.NewJobState
	}

	// Handle the job based on its state
	switch job.Status.State {
	case lmevalservicev1beta1.NewJobState:
		// Handle newly created job
		return r.handleNewCR(ctx, log, job)
	case lmevalservicev1beta1.ScheduledJobState:
		// the job's pod has been created and the driver hasn't updated the state yet
		// let's check the pod status and detect pod failure if there is
		// TODO: need a timeout/retry mechanism here to transite to other states
		return r.checkScheduledPod(ctx, log, job)
	case lmevalservicev1beta1.RunningJobState:
		// TODO: need a timeout/retry mechanism here to transite to other states
		return r.checkScheduledPod(ctx, log, job)
	case lmevalservicev1beta1.CompleteJobState:
		return r.handleComplete(ctx, log, job)
	case lmevalservicev1beta1.CancelledJobState:
		return r.handleCancel(ctx, log, job)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LMEvalJobReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Add a runnable to retrieve the settings from the specified configmap
	if err := mgr.Add(manager.RunnableFunc(func(context.Context) error {
		var cm corev1.ConfigMap
		if err := r.Get(
			context.Background(),
			types.NamespacedName{Namespace: r.Namespace, Name: r.ConfigMap},
			&cm); err != nil {

			ctrl.Log.WithName("setup").Error(err,
				"failed to get configmap",
				"namespace", r.Namespace,
				"name", r.ConfigMap)

			return err
		}

		if err := r.constructOptionsFromConfigMap(&cm); err != nil {
			return err
		}
		return nil
	})); err != nil {
		return err
	}

	// watch the pods created by the controller but only for the deletion event
	return ctrl.NewControllerManagedBy(mgr).
		// since we register the finalizer, no need to monitor deletion events
		For(&lmevalservicev1beta1.LMEvalJob{}, builder.WithPredicates(predicate.Funcs{
			// drop deletion events
			DeleteFunc: func(event.DeleteEvent) bool {
				return false
			},
		})).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &lmevalservicev1beta1.LMEvalJob{}),
			builder.WithPredicates(predicate.Funcs{
				// drop all events except deletion
				CreateFunc: func(event.CreateEvent) bool {
					return false
				},
				UpdateFunc: func(event.UpdateEvent) bool {
					return false
				},
				GenericFunc: func(event.GenericEvent) bool {
					return false
				},
			}),
		).
		Complete(r)
}

func (r *LMEvalJobReconciler) constructOptionsFromConfigMap(configmap *corev1.ConfigMap) error {
	r.options.DriverImage = DefaultDriverImage
	r.options.PodImage = DefaultPodImage
	r.options.DriverServiceAccount = DefaultDriverServiceAccount
	r.options.PodCheckingInterval = DefaultPodCheckingInterval
	r.options.ImagePullPolicy = DefaultImagePullPolicy
	log := log.FromContext(context.Background())

	if v, found := configmap.Data[DriverImageKey]; found {
		r.options.DriverImage = v
	}
	if v, found := configmap.Data[PodImageKey]; found {
		r.options.PodImage = v
	}
	if v, found := configmap.Data[DriverServiceAccountKey]; found {
		r.options.DriverServiceAccount = v
	}
	if v, found := configmap.Data[PodCheckingIntervalKey]; found {
		if d, err := time.ParseDuration(v); err == nil {
			r.options.PodCheckingInterval = d
		} else {
			log.Error(err, "failed to parse the configmap", PodCheckingIntervalKey, v)
		}
	}
	if v, found := configmap.Data[ImagePullPolicyKey]; found {
		if p, found := pullPolicyMap[corev1.PullPolicy(v)]; found {
			r.options.ImagePullPolicy = p
		} else {
			log.Error(
				fmt.Errorf("invalid %s value in the configmap: %s", ImagePullPolicyKey, v),
				"use the default value for the ImagePullPolicy instead",
			)
		}
	}
	return nil
}

func (r *LMEvalJobReconciler) handleDeletion(ctx context.Context, job *lmevalservicev1beta1.LMEvalJob, log logr.Logger) (reconcile.Result, error) {
	if controllerutil.ContainsFinalizer(job, lmevalservicev1beta1.FinalizerName) {
		// delete the correspondling pod if needed
		// remove our finalizer from the list and update it.
		if job.Status.State != lmevalservicev1beta1.CompleteJobState ||
			job.Status.Reason != lmevalservicev1beta1.CancelledReason {

			if err := r.deleteJobPod(ctx, job); err != nil && client.IgnoreNotFound(err) != nil {
				log.Error(err, "failed to delete pod of the job")
			}
		}

		controllerutil.RemoveFinalizer(job, lmevalservicev1beta1.FinalizerName)
		if err := r.Update(ctx, job); err != nil {
			return ctrl.Result{}, err
		}
		r.Recorder.Event(job, "Normal", "DetachFinalizer",
			fmt.Sprintf("removed finalizer from LMEvalJob %s in namespace %s",
				job.Name,
				job.Namespace))
		log.Info("Successfully remove the finalizer", "name", job.Name)
	}

	return ctrl.Result{}, nil
}

func (r *LMEvalJobReconciler) handleNewCR(ctx context.Context, log logr.Logger, job *lmevalservicev1beta1.LMEvalJob) (reconcile.Result, error) {
	// If it doesn't contain our finalizer, add it
	if !controllerutil.ContainsFinalizer(job, lmevalservicev1beta1.FinalizerName) {
		controllerutil.AddFinalizer(job, lmevalservicev1beta1.FinalizerName)
		if err := r.Update(ctx, job); err != nil {
			log.Error(err, "unable to update finalizer")
			return ctrl.Result{}, err
		}
		r.Recorder.Event(job, "Normal", "AttachFinalizer",
			fmt.Sprintf("added the finalizer to the LMEvalJob %s in namespace %s",
				job.Name,
				job.Namespace))
		// Since finalizers were updated. Need to fetch the new LMEvalJob
		// End the current reconsile and get revisioned job in next reconsile
		return ctrl.Result{}, nil
	}

	// construct a new pod and create a pod for the job
	currentTime := v1.Now()
	pod := r.createPod(job)
	if err := r.Create(ctx, pod, &client.CreateOptions{}); err != nil {
		// Failed to create the pod. Mark the status as complete with failed
		job.Status.State = lmevalservicev1beta1.CompleteJobState
		job.Status.Reason = lmevalservicev1beta1.FailedReason
		job.Status.Message = err.Error()
		if err := r.Status().Update(ctx, job); err != nil {
			log.Error(err, "unable to update LMEvalJob status for pod creation failure")
		}
		log.Error(err, "Failed to create pod for the LMEvalJob", "name", job.Name)
		return ctrl.Result{}, err
	}

	// Create the pod successfully. Wait for the driver to update the status
	job.Status.State = lmevalservicev1beta1.ScheduledJobState
	job.Status.PodName = pod.Name
	job.Status.LastScheduleTime = &currentTime
	if err := r.Status().Update(ctx, job); err != nil {
		log.Error(err, "unable to update LMEvalJob status (pod creation done)")
		return ctrl.Result{}, err
	}
	r.Recorder.Event(job, "Normal", "PodCreation",
		fmt.Sprintf("the LMEvalJob %s in namespace %s created a pod",
			job.Name,
			job.Namespace))
	log.Info("Successfully create a Pod for the Job")
	// Check the pod after the config interval
	return ctrl.Result{Requeue: true, RequeueAfter: r.options.PodCheckingInterval}, nil
}

func (r *LMEvalJobReconciler) checkScheduledPod(ctx context.Context, log logr.Logger, job *lmevalservicev1beta1.LMEvalJob) (ctrl.Result, error) {
	pod, err := r.getPod(ctx, job)
	if err != nil {
		// a weird state, someone delete the corresponding pod? mark this as CompleteJobState
		// with error message
		job.Status.State = lmevalservicev1beta1.CompleteJobState
		job.Status.Reason = lmevalservicev1beta1.FailedReason
		job.Status.Message = err.Error()
		if err := r.Status().Update(ctx, job); err != nil {
			log.Error(err, "unable to update LMEvalJob status", "state", job.Status.State)
			return ctrl.Result{}, err
		}
		r.Recorder.Event(job, "Warning", "PodMising",
			fmt.Sprintf("the pod for the LMEvalJob %s in namespace %s is gone",
				job.Name,
				job.Namespace))
		log.Error(err, "since the job's pod is gone, mark the job as complete with error result.")
		return ctrl.Result{}, err
	}

	if pod.Status.ContainerStatuses == nil {
		// wait for the pod to initialize and run the containers
		return ctrl.Result{Requeue: true, RequeueAfter: r.options.PodCheckingInterval}, nil
	}

	mainIndex := slices.IndexFunc(pod.Status.ContainerStatuses, func(s corev1.ContainerStatus) bool {
		return s.Name == "main"
	})
	if mainIndex == -1 || pod.Status.ContainerStatuses[mainIndex].LastTerminationState.Terminated == nil {
		// wait for the main container to finish
		return ctrl.Result{Requeue: true, RequeueAfter: r.options.PodCheckingInterval}, nil
	}

	// main container finished. update status
	job.Status.State = lmevalservicev1beta1.CompleteJobState
	if pod.Status.ContainerStatuses[mainIndex].LastTerminationState.Terminated.ExitCode == 0 {
		job.Status.Reason = lmevalservicev1beta1.SucceedReason
	} else {
		job.Status.Reason = lmevalservicev1beta1.FailedReason
		job.Status.Message = pod.Status.ContainerStatuses[mainIndex].LastTerminationState.Terminated.Reason
	}

	err = r.Status().Update(ctx, job)
	if err != nil {
		log.Error(err, "unable to update LMEvalJob status", "state", job.Status.State)
	}
	r.Recorder.Event(job, "Normal", "PodCompleted",
		fmt.Sprintf("The pod for the LMEvalJob %s in namespace %s has completed",
			job.Name,
			job.Namespace))
	return ctrl.Result{}, err
}

func (r *LMEvalJobReconciler) getPod(ctx context.Context, job *lmevalservicev1beta1.LMEvalJob) (*corev1.Pod, error) {
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

func (r *LMEvalJobReconciler) deleteJobPod(ctx context.Context, job *lmevalservicev1beta1.LMEvalJob) error {
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

func (r *LMEvalJobReconciler) handleComplete(ctx context.Context, log logr.Logger, job *lmevalservicev1beta1.LMEvalJob) (ctrl.Result, error) {
	if job.Status.CompleteTime == nil {
		r.Recorder.Event(job, "Normal", "JobCompleted",
			fmt.Sprintf("The LMEvalJob %s in namespace %s has completed",
				job.Name,
				job.Namespace))
		// TODO: final wrap up/clean up
		current := v1.Now()
		job.Status.CompleteTime = &current
		if err := r.Status().Update(ctx, job); err != nil {
			log.Error(err, "failed to update status for completion")
		}
	}
	return ctrl.Result{}, nil
}

func (r *LMEvalJobReconciler) handleCancel(ctx context.Context, log logr.Logger, job *lmevalservicev1beta1.LMEvalJob) (ctrl.Result, error) {
	// delete the pod and update the state to complete
	if _, err := r.getPod(ctx, job); err != nil {
		// pod is gone. update status
		job.Status.State = lmevalservicev1beta1.CompleteJobState
		job.Status.Reason = lmevalservicev1beta1.FailedReason
		job.Status.Message = err.Error()
	} else {
		job.Status.State = lmevalservicev1beta1.CompleteJobState
		job.Status.Reason = lmevalservicev1beta1.CancelledReason
		if err := r.deleteJobPod(ctx, job); err != nil {
			// leave the state as is and retry again
			log.Error(err, "failed to delete pod. scheduled a retry", "interval", r.options.PodCheckingInterval.String())
			return ctrl.Result{Requeue: true, RequeueAfter: r.options.PodCheckingInterval}, err
		}
	}

	err := r.Status().Update(ctx, job)
	if err != nil {
		log.Error(err, "failed to update status for cancellation")
	}
	r.Recorder.Event(job, "Normal", "Cancelled",
		fmt.Sprintf("The LMEvalJob %s in namespace %s has cancelled and changed its state to Complete",
			job.Name,
			job.Namespace))
	return ctrl.Result{}, err
}

func (r *LMEvalJobReconciler) createPod(job *lmevalservicev1beta1.LMEvalJob) *corev1.Pod {
	var allowPrivilegeEscalation = false
	var runAsNonRootUser = true
	var ownerRefController = true
	var runAsUser int64 = 1001030000

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
			InitContainers: []corev1.Container{
				{
					Name:            "driver",
					Image:           r.options.DriverImage,
					ImagePullPolicy: r.options.ImagePullPolicy,
					Command:         []string{DriverPath, "--copy", DestDriverPath},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: &allowPrivilegeEscalation,
						RunAsUser:                &runAsUser,
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{
								"ALL",
							},
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "shared",
							MountPath: "/opt/app-root/src/bin",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:            "main",
					Image:           r.options.PodImage,
					ImagePullPolicy: r.options.ImagePullPolicy,
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
					Command: generateCmd(job),
					Args:    generateArgs(job),
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: &allowPrivilegeEscalation,
						RunAsUser:                &runAsUser,
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{
								"ALL",
							},
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "shared",
							MountPath: "/opt/app-root/src/bin",
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
			ServiceAccountName: r.options.DriverServiceAccount,
			Volumes: []corev1.Volume{
				{
					Name: "shared", VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}
	return &pod
}

func generateArgs(job *lmevalservicev1beta1.LMEvalJob) []string {
	if job == nil {
		return nil
	}

	cmds := make([]string, 0, 10)
	cmds = append(cmds, "python", "-m", "lm_eval", "--output_path", "/opt/app-root/src/output")
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

	return []string{"sh", "-ec", strings.Join(cmds, " ")}
}

func generateCmd(job *lmevalservicev1beta1.LMEvalJob) []string {
	if job == nil {
		return nil
	}

	return []string{
		DestDriverPath,
		"--job-namespace", job.Namespace,
		"--job-name", job.Name,
		"--output-path", "/opt/app-root/src/output",
		"--",
	}
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
