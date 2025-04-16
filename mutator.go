package main

import (
	"log/slog"
	"strings"

	"golang.org/x/exp/slices"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	kwhmutating "github.com/slok/kubewebhook/v2/pkg/webhook/mutating"

	"github.com/metal-stack/metal-lib/pkg/pointer"
)

const (
	gardenerResourceManager string = "gardener-resource-manager"
	gardenlet               string = "gardenlet"
	nginxIngressController  string = "nginx-ingress-controller"
	singleNodeSeed          string = "single-node-seed"
)

func allMutations() []string {
	return []string{
		gardenerResourceManager,
		gardenlet,
		nginxIngressController,
		singleNodeSeed,
	}
}

type mutator struct {
	cfg *config
	log *slog.Logger
}

func (m *mutator) mutateDeployment(deployment *appsv1.Deployment) (*kwhmutating.MutatorResult, error) {
	if m.mutationIsEnabled(nginxIngressController) && deployment.Name == "nginx-ingress-controller" && deployment.Namespace == "garden" {
		for i, c := range deployment.Spec.Template.Spec.Containers {
			if c.Name == "nginx-ingress-controller" {
				m.log.Info("patching nginx-ingress-controller liveness probe")

				c.LivenessProbe.InitialDelaySeconds = 600

				if strings.Contains(c.Image, "/ingress-nginx/controller-chroot:") && !slices.Contains(c.SecurityContext.Capabilities.Add, "SYS_CHROOT") {
					m.log.Info("patching nginx-ingress-controller with chroot image missing SYS_CHROOT capability")

					c.SecurityContext.Capabilities.Add = append(c.SecurityContext.Capabilities.Add, "SYS_CHROOT")
				}

				deployment.Spec.Template.Spec.Containers[i] = c
			}
		}
	}

	if m.mutationIsEnabled(gardenlet) && deployment.Name == "gardenlet" && deployment.Namespace == "garden" {
		m.log.Info("patching gardenlet pod security context")

		deployment.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			FSGroup: pointer.Pointer(int64(65534)),
		}
	}

	if m.mutationIsEnabled(gardenerResourceManager) && deployment.Name == "gardener-resource-manager" && deployment.Namespace == "garden" {
		m.log.Info("patching gardener-resource-manager readiness probe")

		deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = nil
	}

	if m.mutationIsEnabled(singleNodeSeed) {
		if deployment.Name == "gardener-extension-provider-gcp" {
			m.log.Info("removing provider-gcp pod anti affinity rule")

			deployment.Spec.Template.Spec.Affinity.PodAntiAffinity = nil
		}

		for i, topologySpread := range deployment.Spec.Template.Spec.TopologySpreadConstraints {
			if topologySpread.WhenUnsatisfiable == corev1.DoNotSchedule {
				deployment.Spec.Template.Spec.TopologySpreadConstraints[i].WhenUnsatisfiable = corev1.ScheduleAnyway
				deployment.Spec.Template.Spec.TopologySpreadConstraints[i].MinDomains = nil

				m.log.Info("patching topology do not schedule constraint for single node seed to schedule anyway")
			}
		}
	}

	return &kwhmutating.MutatorResult{MutatedObject: deployment}, nil
}

func (m *mutator) mutateStatefulSet(sts *appsv1.StatefulSet) (*kwhmutating.MutatorResult, error) {
	if m.mutationIsEnabled(singleNodeSeed) {
		for i, topologySpread := range sts.Spec.Template.Spec.TopologySpreadConstraints {
			if topologySpread.WhenUnsatisfiable == corev1.DoNotSchedule {
				sts.Spec.Template.Spec.TopologySpreadConstraints[i].WhenUnsatisfiable = corev1.ScheduleAnyway
				sts.Spec.Template.Spec.TopologySpreadConstraints[i].MinDomains = nil

				m.log.Info("patching topology do not schedule constraint for single node seed to schedule anyway")
			}
		}
	}

	return &kwhmutating.MutatorResult{MutatedObject: sts}, nil
}

func (c *mutator) mutationIsEnabled(m string) bool {
	return slices.Contains(c.cfg.mutations, m)
}
