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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lmevalservicev1beta1 "github.com/foundation-model-stack/fms-lm-eval-service/api/v1beta1"
)

var (
	isController                   = true
	allowPrivilegeEscalation       = false
	runAsNonRootUser               = true
	runAsUser                int64 = 1001030000
	secretMode               int32 = 420
)

var _ = Describe("LMEvalJob Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		evaljob := &lmevalservicev1beta1.LMEvalJob{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind LMEvalJob")
			err := k8sClient.Get(ctx, typeNamespacedName, evaljob)
			if err != nil && errors.IsNotFound(err) {
				resource := &lmevalservicev1beta1.LMEvalJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: lmevalservicev1beta1.LMEvalJobSpec{
						Model: "test",
						ModelArgs: []lmevalservicev1beta1.Arg{
							{Name: "arg1", Value: "value1"},
						},
						Tasks: []string{"task1", "task2"},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &lmevalservicev1beta1.LMEvalJob{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance LMEvalJob")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &LMEvalJobReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func Test_SimplePod(t *testing.T) {
	lmevalRec := LMEvalJobReconciler{
		Namespace: "test",
		options: &ServiceOptions{
			PodImage:        "podimage:latest",
			DriverImage:     "driver:latest",
			ImagePullPolicy: corev1.PullAlways,
			GrpcPort:        8088,
			GrpcService:     "grpc-service",
		},
	}
	var job = &lmevalservicev1beta1.LMEvalJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			UID:       "for-testing",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       lmevalservicev1beta1.KindName,
			APIVersion: lmevalservicev1beta1.Version,
		},
		Spec: lmevalservicev1beta1.LMEvalJobSpec{
			Model: "test",
			ModelArgs: []lmevalservicev1beta1.Arg{
				{Name: "arg1", Value: "value1"},
			},
			Tasks: []string{"task1", "task2"},
		},
	}

	expect := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name": "fms-lm-eval-service",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: lmevalservicev1beta1.Version,
					Kind:       lmevalservicev1beta1.KindName,
					Name:       "test",
					Controller: &isController,
					UID:        "for-testing",
				},
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:            "driver",
					Image:           lmevalRec.options.DriverImage,
					ImagePullPolicy: lmevalRec.options.ImagePullPolicy,
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
					Image:           lmevalRec.options.PodImage,
					ImagePullPolicy: lmevalRec.options.ImagePullPolicy,
					Env:             []corev1.EnvVar{},
					Command:         lmevalRec.generateCmd(job),
					Args:            generateArgs(job),
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

	newPod := lmevalRec.createPod(job)

	assert.Equal(t, expect, newPod)
}

func Test_GrpcMTlsPod(t *testing.T) {
	lmevalRec := LMEvalJobReconciler{
		Namespace: "test",
		options: &ServiceOptions{
			PodImage:         "podimage:latest",
			DriverImage:      "driver:latest",
			ImagePullPolicy:  corev1.PullAlways,
			GrpcPort:         8088,
			GrpcService:      "grpc-service",
			GrpcServerSecret: "server-secret",
			GrpcClientSecret: "client-secret",
			grpcTLSMode:      TLSMode_mTLS,
		},
	}
	var job = &lmevalservicev1beta1.LMEvalJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			UID:       "for-testing",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       lmevalservicev1beta1.KindName,
			APIVersion: lmevalservicev1beta1.Version,
		},
		Spec: lmevalservicev1beta1.LMEvalJobSpec{
			Model: "test",
			ModelArgs: []lmevalservicev1beta1.Arg{
				{Name: "arg1", Value: "value1"},
			},
			Tasks: []string{"task1", "task2"},
		},
	}

	expect := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name": "fms-lm-eval-service",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: lmevalservicev1beta1.Version,
					Kind:       lmevalservicev1beta1.KindName,
					Name:       "test",
					Controller: &isController,
					UID:        "for-testing",
				},
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:            "driver",
					Image:           lmevalRec.options.DriverImage,
					ImagePullPolicy: lmevalRec.options.ImagePullPolicy,
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
					Image:           lmevalRec.options.PodImage,
					ImagePullPolicy: lmevalRec.options.ImagePullPolicy,
					Env: []corev1.EnvVar{
						{
							Name:  "GRPC_CLIENT_KEY",
							Value: "/tmp/k8s-grpc-client/certs/tls.key",
						},
						{
							Name:  "GRPC_CLIENT_CERT",
							Value: "/tmp/k8s-grpc-client/certs/tls.crt",
						},
						{
							Name:  "GRPC_SERVER_CA",
							Value: "/tmp/k8s-grpc-server/certs/ca.crt",
						},
					},
					Command: lmevalRec.generateCmd(job),
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
						{
							Name:      "client-cert",
							MountPath: "/tmp/k8s-grpc-client/certs",
							ReadOnly:  true,
						},
						{
							Name:      "server-cert",
							MountPath: "/tmp/k8s-grpc-server/certs",
							ReadOnly:  true,
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
			Volumes: []corev1.Volume{
				{
					Name: "shared", VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "client-cert",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  lmevalRec.options.GrpcClientSecret,
							DefaultMode: &secretMode,
						},
					},
				},
				{
					Name: "server-cert",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  lmevalRec.options.GrpcServerSecret,
							DefaultMode: &secretMode,
							Items: []corev1.KeyToPath{
								{Key: "ca.crt", Path: "ca.crt"},
							},
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	newPod := lmevalRec.createPod(job)

	assert.Equal(t, expect, newPod)
}

func Test_EnvSecretsPod(t *testing.T) {
	lmevalRec := LMEvalJobReconciler{
		Namespace: "test",
		options: &ServiceOptions{
			PodImage:         "podimage:latest",
			DriverImage:      "driver:latest",
			ImagePullPolicy:  corev1.PullAlways,
			GrpcPort:         8088,
			GrpcService:      "grpc-service",
			GrpcServerSecret: "server-secret",
			GrpcClientSecret: "client-secret",
			grpcTLSMode:      TLSMode_mTLS,
		},
	}
	var job = &lmevalservicev1beta1.LMEvalJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			UID:       "for-testing",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       lmevalservicev1beta1.KindName,
			APIVersion: lmevalservicev1beta1.Version,
		},
		Spec: lmevalservicev1beta1.LMEvalJobSpec{
			Model: "test",
			ModelArgs: []lmevalservicev1beta1.Arg{
				{Name: "arg1", Value: "value1"},
			},
			Tasks: []string{"task1", "task2"},
			EnvSecrets: []lmevalservicev1beta1.EnvSecret{
				{
					Env: "my_env",
					SecretRef: &corev1.SecretKeySelector{
						Key: "my-key",
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "my-secret",
						},
					},
				},
			},
		},
	}

	expect := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name": "fms-lm-eval-service",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: lmevalservicev1beta1.Version,
					Kind:       lmevalservicev1beta1.KindName,
					Name:       "test",
					Controller: &isController,
					UID:        "for-testing",
				},
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:            "driver",
					Image:           lmevalRec.options.DriverImage,
					ImagePullPolicy: lmevalRec.options.ImagePullPolicy,
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
					Image:           lmevalRec.options.PodImage,
					ImagePullPolicy: lmevalRec.options.ImagePullPolicy,
					Env: []corev1.EnvVar{
						{
							Name: "my_env",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									Key: "my-key",
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "my-secret",
									},
								},
							},
						},
						{
							Name:  "GRPC_CLIENT_KEY",
							Value: "/tmp/k8s-grpc-client/certs/tls.key",
						},
						{
							Name:  "GRPC_CLIENT_CERT",
							Value: "/tmp/k8s-grpc-client/certs/tls.crt",
						},
						{
							Name:  "GRPC_SERVER_CA",
							Value: "/tmp/k8s-grpc-server/certs/ca.crt",
						},
					},
					Command: lmevalRec.generateCmd(job),
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
						{
							Name:      "client-cert",
							MountPath: "/tmp/k8s-grpc-client/certs",
							ReadOnly:  true,
						},
						{
							Name:      "server-cert",
							MountPath: "/tmp/k8s-grpc-server/certs",
							ReadOnly:  true,
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
			Volumes: []corev1.Volume{
				{
					Name: "shared", VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "client-cert",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  lmevalRec.options.GrpcClientSecret,
							DefaultMode: &secretMode,
						},
					},
				},
				{
					Name: "server-cert",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  lmevalRec.options.GrpcServerSecret,
							DefaultMode: &secretMode,
							Items: []corev1.KeyToPath{
								{Key: "ca.crt", Path: "ca.crt"},
							},
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	newPod := lmevalRec.createPod(job)
	// maybe only verify the envs: Containers[0].Env
	assert.Equal(t, expect, newPod)
}

func Test_FileSecretsPod(t *testing.T) {
	lmevalRec := LMEvalJobReconciler{
		Namespace: "test",
		options: &ServiceOptions{
			PodImage:         "podimage:latest",
			DriverImage:      "driver:latest",
			ImagePullPolicy:  corev1.PullAlways,
			GrpcPort:         8088,
			GrpcService:      "grpc-service",
			GrpcServerSecret: "server-secret",
			GrpcClientSecret: "client-secret",
			grpcTLSMode:      TLSMode_mTLS,
		},
	}
	var job = &lmevalservicev1beta1.LMEvalJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			UID:       "for-testing",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       lmevalservicev1beta1.KindName,
			APIVersion: lmevalservicev1beta1.Version,
		},
		Spec: lmevalservicev1beta1.LMEvalJobSpec{
			Model: "test",
			ModelArgs: []lmevalservicev1beta1.Arg{
				{Name: "arg1", Value: "value1"},
			},
			Tasks: []string{"task1", "task2"},
			FileSecrets: []lmevalservicev1beta1.FileSecret{
				{
					MountPath: "the_path",
					SecretRef: corev1.SecretVolumeSource{
						SecretName: "my-secret",
						Items: []corev1.KeyToPath{
							{
								Key:  "key1",
								Path: "path1",
							},
						},
					},
				},
			},
		},
	}

	expect := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Labels: map[string]string{
				"app.kubernetes.io/name": "fms-lm-eval-service",
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: lmevalservicev1beta1.Version,
					Kind:       lmevalservicev1beta1.KindName,
					Name:       "test",
					Controller: &isController,
					UID:        "for-testing",
				},
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:            "driver",
					Image:           lmevalRec.options.DriverImage,
					ImagePullPolicy: lmevalRec.options.ImagePullPolicy,
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
					Image:           lmevalRec.options.PodImage,
					ImagePullPolicy: lmevalRec.options.ImagePullPolicy,
					Env: []corev1.EnvVar{
						{
							Name:  "GRPC_CLIENT_KEY",
							Value: "/tmp/k8s-grpc-client/certs/tls.key",
						},
						{
							Name:  "GRPC_CLIENT_CERT",
							Value: "/tmp/k8s-grpc-client/certs/tls.crt",
						},
						{
							Name:  "GRPC_SERVER_CA",
							Value: "/tmp/k8s-grpc-server/certs/ca.crt",
						},
					},
					Command: lmevalRec.generateCmd(job),
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
						{
							Name:      "client-cert",
							MountPath: "/tmp/k8s-grpc-client/certs",
							ReadOnly:  true,
						},
						{
							Name:      "server-cert",
							MountPath: "/tmp/k8s-grpc-server/certs",
							ReadOnly:  true,
						},
						{
							Name:      "secVol1",
							MountPath: "the_path",
							ReadOnly:  true,
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
			Volumes: []corev1.Volume{
				{
					Name: "shared", VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: "client-cert",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  lmevalRec.options.GrpcClientSecret,
							DefaultMode: &secretMode,
						},
					},
				},
				{
					Name: "server-cert",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName:  lmevalRec.options.GrpcServerSecret,
							DefaultMode: &secretMode,
							Items: []corev1.KeyToPath{
								{Key: "ca.crt", Path: "ca.crt"},
							},
						},
					},
				},
				{
					Name: "secVol1",
					VolumeSource: corev1.VolumeSource{
						Secret: &corev1.SecretVolumeSource{
							SecretName: "my-secret",
							Items: []corev1.KeyToPath{
								{
									Key:  "key1",
									Path: "path1",
								},
							},
						},
					},
				},
			},
			RestartPolicy: corev1.RestartPolicyNever,
		},
	}

	newPod := lmevalRec.createPod(job)
	// maybe only verify the envs: Containers[0].Env
	assert.Equal(t, expect, newPod)
}
