package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/exp/slices"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kwhhttp "github.com/slok/kubewebhook/v2/pkg/http"
	kwhmodel "github.com/slok/kubewebhook/v2/pkg/model"
	kwhmutating "github.com/slok/kubewebhook/v2/pkg/webhook/mutating"

	"github.com/metal-stack/metal-lib/pkg/pointer"
)

type config struct {
	certFile  string
	keyFile   string
	mutations []string
}

func initFlags() (*config, error) {
	cfg := &config{}

	fl := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fl.StringVar(&cfg.certFile, "tls-cert-file", "/etc/metal-seed-mutator/cert.pem", "TLS certificate file")
	fl.StringVar(&cfg.keyFile, "tls-key-file", "/etc/metal-seed-mutator/key.pem", "TLS key file")
	mutations := fl.String("mutations", "nginx-ingress-controller", "the mutations to apply (comma-separated, can be nginx-ingress-controller|gardenlet|single-node-seed|gardener-resource-manager)")

	err := fl.Parse(os.Args[1:])
	if err != nil {
		return nil,
			err
	}

	if mutations != nil {
		cfg.mutations = strings.Split(*mutations, ",")
	}

	return cfg, nil
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg, err := initFlags()
	if err != nil {
		return err
	}

	// Create mutator.
	mt := kwhmutating.MutatorFunc(func(_ context.Context, _ *kwhmodel.AdmissionReview, obj metav1.Object) (*kwhmutating.MutatorResult, error) {
		switch o := obj.(type) {
		case *appsv1.Deployment:
			logger.Info("mutating deployment %s/%s", o.Namespace, o.Name)
			return mutateDeployment(logger, cfg, o)
		case *appsv1.StatefulSet:
			logger.Info("mutating stateful set %s/%s", o.Namespace, o.Name)
			return mutateStatefulSet(logger, cfg, o)
		default:
			return &kwhmutating.MutatorResult{}, nil
		}
	})

	// Create webhook.
	mcfg := kwhmutating.WebhookConfig{
		ID:      "metal-seed-mutator.metal-stack.dev",
		Mutator: mt,
		Logger:  nil,
	}
	wh, err := kwhmutating.NewWebhook(mcfg)
	if err != nil {
		return fmt.Errorf("error creating webhook: %w", err)
	}

	// Get HTTP handler from webhook.
	whHandler, err := kwhhttp.HandlerFor(kwhhttp.HandlerConfig{Webhook: wh})
	if err != nil {
		return fmt.Errorf("error creating webhook handler: %w", err)
	}

	// Serve.
	logger.Info("Listening on :8080")

	server := &http.Server{
		Addr:              ":8080",
		Handler:           whHandler,
		ReadHeaderTimeout: 1 * time.Minute,
	}

	err = server.ListenAndServeTLS(cfg.certFile, cfg.keyFile)
	if err != nil {
		return fmt.Errorf("error serving webhook: %w", err)
	}

	return nil
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running app: %s", err)
		os.Exit(1)
	}
}

func mutateDeployment(logger *slog.Logger, cfg *config, deployment *appsv1.Deployment) (*kwhmutating.MutatorResult, error) {
	if deployment.Name == "nginx-ingress-controller" && deployment.Namespace == "garden" {
		containers := deployment.Spec.Template.Spec.Containers
		for i, c := range containers {
			if slices.Contains(cfg.mutations, "nginx-ingress-controller") && c.Name == "nginx-ingress-controller" {
				logger.Info("patching nginx-ingress-controller liveness probe")

				c.LivenessProbe.InitialDelaySeconds = 600

				if strings.Contains(c.Image, "/ingress-nginx/controller-chroot:") && !slices.Contains(c.SecurityContext.Capabilities.Add, "SYS_CHROOT") {
					logger.Info("patching nginx-ingress-controller with chroot image missing SYS_CHROOT capability")

					c.SecurityContext.Capabilities.Add = append(c.SecurityContext.Capabilities.Add, "SYS_CHROOT")
				}

				deployment.Spec.Template.Spec.Containers[i] = c
			}
		}
	}

	if slices.Contains(cfg.mutations, "gardenlet") && deployment.Name == "gardenlet" && deployment.Namespace == "garden" {
		logger.Info("patching gardenlet pod security context")

		deployment.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
			FSGroup: pointer.Pointer(int64(65534)),
		}
	}

	if slices.Contains(cfg.mutations, "gardener-resource-manager") && deployment.Name == "gardener-resource-manager" && deployment.Namespace == "garden" {
		logger.Info("patching gardener-resource-manager readiness probe")

		deployment.Spec.Template.Spec.Containers[0].ReadinessProbe = nil
	}

	if slices.Contains(cfg.mutations, "single-node-seed") {
		if deployment.Name == "gardener-extension-provider-gcp" {
			logger.Info("removing provider-gcp pod anti affinity rule")

			deployment.Spec.Template.Spec.Affinity.PodAntiAffinity = nil
		}

		for i, topologySpread := range deployment.Spec.Template.Spec.TopologySpreadConstraints {
			if topologySpread.WhenUnsatisfiable == corev1.DoNotSchedule {
				deployment.Spec.Template.Spec.TopologySpreadConstraints[i].WhenUnsatisfiable = corev1.ScheduleAnyway

				logger.Info("patching topology do not schedule constraint for single node seed to schedule anyway")
			}
		}
	}

	return &kwhmutating.MutatorResult{MutatedObject: deployment}, nil
}

func mutateStatefulSet(logger *slog.Logger, cfg *config, sts *appsv1.StatefulSet) (*kwhmutating.MutatorResult, error) {
	if slices.Contains(cfg.mutations, "single-node-seed") {
		for i, topologySpread := range sts.Spec.Template.Spec.TopologySpreadConstraints {
			if topologySpread.WhenUnsatisfiable == corev1.DoNotSchedule {
				sts.Spec.Template.Spec.TopologySpreadConstraints[i].WhenUnsatisfiable = corev1.ScheduleAnyway

				logger.Info("patching topology do not schedule constraint for single node seed to schedule anyway")
			}
		}
	}

	return &kwhmutating.MutatorResult{MutatedObject: sts}, nil
}
