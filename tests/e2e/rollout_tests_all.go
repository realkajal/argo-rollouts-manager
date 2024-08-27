package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/argoproj-labs/argo-rollouts-manager/tests/e2e/fixture"

	"github.com/argoproj-labs/argo-rollouts-manager/tests/e2e/fixture/k8s"
	rolloutManagerFixture "github.com/argoproj-labs/argo-rollouts-manager/tests/e2e/fixture/rolloutmanager"
	monitoringv1 "github.com/coreos/prometheus-operator/pkg/apis/monitoring/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	rolloutsmanagerv1alpha1 "github.com/argoproj-labs/argo-rollouts-manager/api/v1alpha1"

	controllers "github.com/argoproj-labs/argo-rollouts-manager/controllers"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This file contains tests that should run in both namespace-scoped and cluster-scoped scenarios.
// As of this writing, these function is called from the 'tests/e2e/(cluster-scoped/namespace-scoped)' packages.
func RunRolloutsTests(namespaceScopedParam bool) {

	testType := "cluster-scoped"
	if namespaceScopedParam {
		testType = "namespace-scoped"
	}

	Context("RolloutManager tests - "+testType, func() {

		var (
			k8sClient      client.Client
			ctx            context.Context
			rolloutManager rolloutsmanagerv1alpha1.RolloutManager
		)

		BeforeEach(func() {
			Expect(fixture.EnsureCleanSlate()).To(Succeed())

			var err error
			k8sClient, _, err = fixture.GetE2ETestKubeClient()
			Expect(err).ToNot(HaveOccurred())
			ctx = context.Background()

			rolloutManager = rolloutsmanagerv1alpha1.RolloutManager{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic-rollouts-manager",
					Namespace: fixture.TestE2ENamespace,
				},
				Spec: rolloutsmanagerv1alpha1.RolloutManagerSpec{
					NamespaceScoped: namespaceScopedParam,
				},
			}
		})

		When("Reconcile is called on a new, basic, namespaced-scoped RolloutManager", func() {
			It("should create the appropriate K8s resources", func() {
				Expect(k8sClient.Create(ctx, &rolloutManager)).To(Succeed())

				By("waiting for phase to be \"Available\"")
				Eventually(rolloutManager, "60s", "1s").Should(rolloutManagerFixture.HavePhase(rolloutsmanagerv1alpha1.PhaseAvailable))

				By("Verify that expected resources are created.")
				ValidateArgoRolloutManagerResources(ctx, rolloutManager, k8sClient, namespaceScopedParam)
			})
		})

		When("A RolloutManager is deleted", func() {
			It("should delete all the associated resources", func() {
				Expect(k8sClient.Create(ctx, &rolloutManager)).To(Succeed())
				Eventually(rolloutManager, "60s", "1s").Should(rolloutManagerFixture.HavePhase(rolloutsmanagerv1alpha1.PhaseAvailable))

				Expect(k8sClient.Delete(ctx, &rolloutManager)).To(Succeed())

				By("deleting the service account")
				Eventually(&corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rolloutManager.Namespace},
				}, "10s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

				if namespaceScopedParam {
					By("deleting the role")
					Eventually(&rbacv1.Role{
						ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rolloutManager.Namespace},
					}, "10s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

					By("deleting the role binding")
					Eventually(&rbacv1.RoleBinding{
						ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rolloutManager.Namespace},
					}, "10s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

				} else {
					By("deleting the cluster role")
					Eventually(&rbacv1.ClusterRole{
						ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName},
					}, "10s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

					By("deleting the cluster role binding")
					Eventually(&rbacv1.ClusterRoleBinding{
						ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName},
					}, "10s", "1s").ShouldNot(k8s.ExistByName(k8sClient))
				}

				By("deleting the deployment")
				Eventually(&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rolloutManager.Namespace},
				}, "10s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

				By("deleting the service")
				Eventually(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsMetricsServiceName, Namespace: rolloutManager.Namespace},
				}, "10s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

				By("deleting the secret")
				Eventually(&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultRolloutsNotificationSecretName, Namespace: rolloutManager.Namespace},
				}, "30s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

				By("deleting the serviceMonitor")
				Eventually(&monitoringv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rolloutManager.Namespace},
				}, "30s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

				By("deleting three aggregate cluster roles")
				clusterRoleSuffixes := []string{"aggregate-to-admin", "aggregate-to-edit", "aggregate-to-view"}
				for _, suffix := range clusterRoleSuffixes {
					clusterRoleName := "argo-rollouts-" + suffix
					Consistently(&rbacv1.ClusterRole{
						ObjectMeta: metav1.ObjectMeta{Name: clusterRoleName},
					}, "5s", "1s").ShouldNot(k8s.ExistByName(k8sClient))
				}
			})
		})

		When("A RolloutManager specifies an extra argument", func() {
			It("should reflect that argument in the deployment", func() {
				By("creating the deployment with the argument from the RolloutManager")
				rolloutManager.Spec = rolloutsmanagerv1alpha1.RolloutManagerSpec{
					ExtraCommandArgs: []string{
						"--loglevel",
						"error",
					},
					NamespaceScoped: namespaceScopedParam,
				}
				Expect(k8sClient.Create(ctx, &rolloutManager)).To(Succeed())
				Eventually(rolloutManager, "1m", "1s").Should(rolloutManagerFixture.HavePhase(rolloutsmanagerv1alpha1.PhaseAvailable))

				deployment := appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rolloutManager.Namespace},
				}
				Eventually(&deployment, "10s", "1s").Should(k8s.ExistByName(k8sClient))

				var expectedContainerArgs []string
				if namespaceScopedParam {
					expectedContainerArgs = []string{"--namespaced", "--loglevel", "error"}
				} else {
					expectedContainerArgs = []string{"--loglevel", "error"}
				}

				Expect(deployment.Spec.Template.Spec.Containers[0].Args).To(Equal(expectedContainerArgs))

				By("updating the deployment when the argument in the RolloutManager is updated")

				err := k8s.UpdateWithoutConflict(ctx, &rolloutManager, k8sClient, func(obj client.Object) {
					goObj, ok := obj.(*rolloutsmanagerv1alpha1.RolloutManager)
					Expect(ok).To(BeTrue())

					goObj.Spec = rolloutsmanagerv1alpha1.RolloutManagerSpec{
						ExtraCommandArgs: []string{
							"--logformat",
							"text",
						},
						NamespaceScoped: namespaceScopedParam,
					}
				})
				Expect(err).ToNot(HaveOccurred())

				if namespaceScopedParam {
					expectedContainerArgs = []string{"--namespaced", "--logformat", "text"}
				} else {
					expectedContainerArgs = []string{"--logformat", "text"}
				}

				Eventually(func() []string {
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&deployment), &deployment)).To(Succeed())
					return deployment.Spec.Template.Spec.Containers[0].Args
				}, "10s", "1s").Should(Equal(expectedContainerArgs))
			})
		})

		When("A RolloutManager specifies environment variables", func() {
			It("should reflect those variables in the deployment", func() {
				By("creating the deployment with the environment variables specified in the RolloutManager")

				rolloutManager.Spec.Env = []corev1.EnvVar{
					{Name: "EDITOR", Value: "emacs"},
					{Name: "LANG", Value: "en_CA.UTF-8"},
				}

				Expect(k8sClient.Create(ctx, &rolloutManager)).To(Succeed())
				Eventually(rolloutManager, "1m", "1s").Should(rolloutManagerFixture.HavePhase(rolloutsmanagerv1alpha1.PhaseAvailable))

				deployment := appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rolloutManager.Namespace},
				}
				Eventually(&deployment, "10s", "1s").Should(k8s.ExistByName(k8sClient))
				Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(SatisfyAll(
					HaveLen(2),
					ContainElements(
						corev1.EnvVar{Name: "EDITOR", Value: "emacs"},
						corev1.EnvVar{Name: "LANG", Value: "en_CA.UTF-8"},
					),
				))

				By("updating the deployment when the environment variables in the RolloutManager are updated")

				err := k8s.UpdateWithoutConflict(ctx, &rolloutManager, k8sClient, func(obj client.Object) {
					goObj, ok := obj.(*rolloutsmanagerv1alpha1.RolloutManager)
					Expect(ok).To(BeTrue())

					goObj.Spec.Env = []corev1.EnvVar{
						{Name: "LANG", Value: "en_US.UTF-8"},
						{Name: "TERM", Value: "xterm-256color"},
					}
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() []corev1.EnvVar {
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&deployment), &deployment)).To(Succeed())
					return deployment.Spec.Template.Spec.Containers[0].Env
				}, "10s", "1s").Should(SatisfyAll(
					HaveLen(2),
					ContainElements(
						corev1.EnvVar{Name: "LANG", Value: "en_US.UTF-8"},
						corev1.EnvVar{Name: "TERM", Value: "xterm-256color"},
					),
				))
			})
		})

		When("A RolloutManager specifies an image", func() {
			It("should reflect that image in the deployment", func() {
				By("creating the deployment with the image specified in the RolloutManager")

				rolloutManager.Spec.Image = "quay.io/prometheus/busybox"
				rolloutManager.Spec.Version = "latest"

				Expect(k8sClient.Create(ctx, &rolloutManager)).To(Succeed())
				Eventually(rolloutManager, "1m", "1s").Should(rolloutManagerFixture.HavePhase(rolloutsmanagerv1alpha1.PhasePending))

				deployment := appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rolloutManager.Namespace},
				}
				Eventually(&deployment, "10s", "1s").Should(k8s.ExistByName(k8sClient))
				expectedVersion := rolloutManager.Spec.Image + ":" + rolloutManager.Spec.Version
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&deployment), &deployment)).To(Succeed())
				Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(expectedVersion))

				By("updating the deployment when the image in the RolloutManager is updated")

				err := k8s.UpdateWithoutConflict(ctx, &rolloutManager, k8sClient, func(obj client.Object) {
					goObj, ok := obj.(*rolloutsmanagerv1alpha1.RolloutManager)
					Expect(ok).To(BeTrue())
					goObj.Spec.Image = controllers.DefaultArgoRolloutsImage
					goObj.Spec.Version = controllers.DefaultArgoRolloutsVersion

				})
				Expect(err).ToNot(HaveOccurred())

				expectedVersion = controllers.DefaultArgoRolloutsImage + ":" + controllers.DefaultArgoRolloutsVersion
				Eventually(func() string {
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&deployment), &deployment)).To(Succeed())
					return deployment.Spec.Template.Spec.Containers[0].Image
				}, "10s", "1s").Should(Equal(expectedVersion))

				expectedServiceMonitor := &monitoringv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      controllers.DefaultArgoRolloutsResourceName,
						Namespace: fixture.TestE2ENamespace,
					},
					Spec: monitoringv1.ServiceMonitorSpec{
						Selector: metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/name": controllers.DefaultArgoRolloutsMetricsServiceName,
							},
						},
						Endpoints: []monitoringv1.Endpoint{
							{
								Port: "metrics",
							},
						},
					},
				}

				By("verify whether ServiceMonitor is created or not for RolloutManager")
				sm := &monitoringv1.ServiceMonitor{
					ObjectMeta: metav1.ObjectMeta{
						Name:      controllers.DefaultArgoRolloutsResourceName,
						Namespace: fixture.TestE2ENamespace,
					},
				}

				Eventually(sm, "10s", "1s").Should(k8s.ExistByName(k8sClient))
				Expect(sm.Name).To(Equal(expectedServiceMonitor.Name))
				Expect(sm.Namespace).To(Equal(expectedServiceMonitor.Namespace))
				Expect(sm.Spec).To(Equal(expectedServiceMonitor.Spec))

			})
		})

		When("a label or annotation is added to Rollout's Deployment after the Deployment has been created", func() {

			It("should not ovewrite the label/annotation with operator labels/annotations, and should instead merge them", func() {

				By("creating default RolloutManager")
				Expect(k8sClient.Create(ctx, &rolloutManager)).To(Succeed())
				Eventually(rolloutManager, "1m", "1s").Should(rolloutManagerFixture.HavePhase(rolloutsmanagerv1alpha1.PhaseAvailable))

				deployment := appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      controllers.DefaultArgoRolloutsResourceName,
						Namespace: rolloutManager.Namespace,
					},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&deployment), &deployment)).To(Succeed())

				deploymentExistingLabels := deployment.DeepCopy().GetLabels()
				deploymentExistingAnnotations := deployment.DeepCopy().GetAnnotations()

				By("updating the default Rollouts deployment with new labels")

				Expect(k8s.UpdateWithoutConflict(ctx, &deployment, k8sClient, func(o client.Object) {

					annots := o.GetAnnotations()
					annots["new-annotation"] = "new-annotation-value"
					o.SetAnnotations(annots)

					labels := o.GetLabels()
					labels["new-label"] = "new-label-value"
					o.SetLabels(labels)

				})).To(Succeed())

				Consistently(&deployment, "10s", "1s").Should(k8s.HaveLabel("new-label", "new-label-value", k8sClient), "user labels should still be present")
				Consistently(&deployment, "10s", "1s").Should(k8s.HaveAnnotation("new-annotation", "new-annotation-value", k8sClient), "user labels should still be present")

				for k, v := range deploymentExistingLabels {
					Expect(&deployment).To(k8s.HaveLabel(k, v, k8sClient), "operator labels should also still be present")
				}
				for k, v := range deploymentExistingAnnotations {
					Expect(&deployment).To(k8s.HaveAnnotation(k, v, k8sClient), "operator labels should also still be present")
				}

				By("removing the used-defined labels from the Deployment")

				Expect(k8s.UpdateWithoutConflict(ctx, &deployment, k8sClient, func(o client.Object) {

					annots := o.GetAnnotations()
					delete(annots, "new-annotation")
					o.SetAnnotations(annots)

					labels := o.GetLabels()
					delete(labels, "new-label")
					o.SetLabels(labels)

				})).To(Succeed())

				for k, v := range deploymentExistingLabels {
					Consistently(&deployment, "5s", "1s").Should(k8s.HaveLabel(k, v, k8sClient), "operator labels should also still be present")
				}
				for k, v := range deploymentExistingAnnotations {
					Consistently(&deployment, "5s", "1s").Should(k8s.HaveAnnotation(k, v, k8sClient), "operator annotations should also still be present")
				}

			})
		})

		When("A RolloutManager specifies metadata", func() {

			It("should create the controller with the correct labels and annotations", func() {

				rolloutsManager := rolloutsmanagerv1alpha1.RolloutManager{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic-rollouts-manager-with-metadata",
						Namespace: fixture.TestE2ENamespace,
					},
					Spec: rolloutsmanagerv1alpha1.RolloutManagerSpec{
						AdditionalMetadata: &rolloutsmanagerv1alpha1.ResourceMetadata{
							Annotations: map[string]string{
								"foo-annotation":  "bar-annotation",
								"foo-annotation2": "bar-annotation2",
							},
							Labels: map[string]string{
								"foo-label":  "bar-label",
								"foo-label2": "bar-label2",
							},
						},
						NamespaceScoped: namespaceScopedParam,
					},
				}

				Expect(k8sClient.Create(ctx, &rolloutsManager)).To(Succeed())

				Eventually(rolloutsManager, "1m", "1s").Should(rolloutManagerFixture.HavePhase(rolloutsmanagerv1alpha1.PhaseAvailable))

				deployment := appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rolloutManager.Namespace},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&deployment), &deployment)).To(Succeed())

				expectMetadataOnObjectMeta(&deployment.ObjectMeta, rolloutsManager.Spec.AdditionalMetadata)
				expectMetadataOnObjectMeta(&deployment.Spec.Template.ObjectMeta, rolloutsManager.Spec.AdditionalMetadata)

				configMap := corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultRolloutsConfigMapName, Namespace: rolloutManager.Namespace},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&configMap), &configMap)).To(Succeed())
				expectMetadataOnObjectMeta(&configMap.ObjectMeta, rolloutsManager.Spec.AdditionalMetadata)

				serviceAccount := corev1.ServiceAccount{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rolloutManager.Namespace},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&serviceAccount), &serviceAccount)).To(Succeed())
				expectMetadataOnObjectMeta(&serviceAccount.ObjectMeta, rolloutsManager.Spec.AdditionalMetadata)

				secret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultRolloutsNotificationSecretName, Namespace: rolloutManager.Namespace},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret)).To(Succeed())
				expectMetadataOnObjectMeta(&secret.ObjectMeta, rolloutsManager.Spec.AdditionalMetadata)

				service := corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsMetricsServiceName, Namespace: rolloutManager.Namespace},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&service), &service)).To(Succeed())
				expectMetadataOnObjectMeta(&service.ObjectMeta, rolloutsManager.Spec.AdditionalMetadata)
			})
		})

		When("A RolloutManager specifies controller resources under .spec.controllerResources", func() {

			It("should create the controller with the correct resources requests/limits", func() {

				By("creating a RolloutManager containing resource requirements")

				rmWithResources := rolloutsmanagerv1alpha1.RolloutManager{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic-rollouts-manager-with-resources",
						Namespace: fixture.TestE2ENamespace,
					},
					Spec: rolloutsmanagerv1alpha1.RolloutManagerSpec{
						ControllerResources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("100Mi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("500m"),
								corev1.ResourceMemory: resource.MustParse("500Mi"),
							},
						},
						NamespaceScoped: namespaceScopedParam,
					},
				}

				Expect(k8sClient.Create(ctx, &rmWithResources)).To(Succeed())

				Eventually(rmWithResources, "1m", "1s").Should(rolloutManagerFixture.HavePhase(rolloutsmanagerv1alpha1.PhaseAvailable))

				deployment := appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rmWithResources.Namespace},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&deployment), &deployment)).To(Succeed())

				Expect(deployment.Spec.Template.Spec.Containers[0].Resources).To(Equal(*rmWithResources.Spec.ControllerResources))

				By("updating RolloutManager to use a different CPU limit")

				err := k8s.UpdateWithoutConflict(ctx, &rmWithResources, k8sClient, func(obj client.Object) {
					rm, ok := obj.(*rolloutsmanagerv1alpha1.RolloutManager)
					Expect(ok).To(BeTrue())

					rm.Spec.ControllerResources.Limits[corev1.ResourceCPU] = resource.MustParse("555m")

				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(func() bool {
					deployment := appsv1.Deployment{
						ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultArgoRolloutsResourceName, Namespace: rmWithResources.Namespace},
					}
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(&deployment), &deployment); err != nil {
						return false
					}
					return deployment.Spec.Template.Spec.Containers[0].Resources.Limits[corev1.ResourceCPU] == resource.MustParse("555m")

				}, "1m", "1s").Should(BeTrue(), "Deployment should switch to the new CPU limit on update of RolloutManager CR")
			})
		})

		DescribeTable("RolloutManager is initially created with a given SkipNotificationSecretDeployment (true/false), then it swaps", func(initialSkipNotificationValue bool) {

			By(fmt.Sprintf("creating RolloutManager with SkipNotificationSecretDeployment set to '%v'", initialSkipNotificationValue))

			rolloutsManager := rolloutsmanagerv1alpha1.RolloutManager{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "basic-rollouts-manager-with-skip-notification-secret",
					Namespace: fixture.TestE2ENamespace,
				},
				Spec: rolloutsmanagerv1alpha1.RolloutManagerSpec{
					NamespaceScoped:                  namespaceScopedParam,
					SkipNotificationSecretDeployment: initialSkipNotificationValue,
				},
			}

			Expect(k8sClient.Create(ctx, &rolloutsManager)).To(Succeed())

			Eventually(rolloutsManager, "1m", "1s").Should(rolloutManagerFixture.HavePhase(rolloutsmanagerv1alpha1.PhaseAvailable))

			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultRolloutsNotificationSecretName, Namespace: rolloutManager.Namespace},
			}
			if rolloutsManager.Spec.SkipNotificationSecretDeployment {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret)).ToNot(Succeed())
			} else {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret)).To(Succeed())
			}

			By(fmt.Sprintf("setting the SkipNotificationSecretDeployment to '%v'", !initialSkipNotificationValue))
			err := k8s.UpdateWithoutConflict(ctx, &rolloutsManager, k8sClient, func(obj client.Object) {
				rmObj, ok := obj.(*rolloutsmanagerv1alpha1.RolloutManager)
				Expect(ok).To(BeTrue())
				rmObj.Spec.SkipNotificationSecretDeployment = !initialSkipNotificationValue
			})
			Expect(err).ToNot(HaveOccurred())

			if rolloutsManager.Spec.SkipNotificationSecretDeployment {
				Eventually(&secret, "10s", "1s").ShouldNot(k8s.ExistByName(k8sClient))

			} else {
				Eventually(&secret, "10s", "1s").Should(k8s.ExistByName(k8sClient))
			}

		},
			Entry("skipNotification is initially true, then set to false", true),
			Entry("skipNotification is initially false, then set to true", false),
		)

		When("A RolloutManager is deleted but the notification secret is owned by another controller", func() {
			It("should not delete the secret", func() {

				rolloutsManager := rolloutsmanagerv1alpha1.RolloutManager{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "basic-rollouts-manager-without-secret",
						Namespace: fixture.TestE2ENamespace,
					},
					Spec: rolloutsmanagerv1alpha1.RolloutManagerSpec{
						NamespaceScoped:                  namespaceScopedParam,
						SkipNotificationSecretDeployment: true,
					},
				}

				Expect(k8sClient.Create(ctx, &rolloutsManager)).To(Succeed())

				Eventually(rolloutsManager, "1m", "1s").Should(rolloutManagerFixture.HavePhase(rolloutsmanagerv1alpha1.PhaseAvailable))

				secret := corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: controllers.DefaultRolloutsNotificationSecretName, Namespace: rolloutManager.Namespace},
				}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret)).ToNot(Succeed())

				By("Creating the secret with another owner")
				secret.OwnerReferences = append(secret.OwnerReferences, metav1.OwnerReference{
					Name:       "another-owner",
					APIVersion: "v1",
					Kind:       "OwnerKind",
					UID:        "1234",
				})
				Expect(k8sClient.Create(ctx, &secret)).To(Succeed())
				Eventually(&secret, "10s", "1s").Should(k8s.ExistByName(k8sClient))

				By("Deleting the RolloutManager")
				Expect(k8sClient.Delete(ctx, &rolloutsManager)).To(Succeed())

				Eventually(&secret, "10s", "1s").Should(k8s.ExistByName(k8sClient))

			})
		})
	})
}
