package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/exp/slices"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/sirupsen/logrus"
	kwhhttp "github.com/slok/kubewebhook/v2/pkg/http"
	kwhlogrus "github.com/slok/kubewebhook/v2/pkg/log/logrus"
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
	mutations := fl.String("mutations", "nginx-ingress-controller", "the mutations to apply (comma-separated, can be nginx-ingress-controller|gardenlet|provider-gcp)")

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
	logrusLogEntry := logrus.NewEntry(logrus.New())
	logrusLogEntry.Logger.SetLevel(logrus.DebugLevel)
	logger := kwhlogrus.NewLogrus(logrusLogEntry)

	cfg, err := initFlags()
	if err != nil {
		return err
	}

	logger.Infof("read flags: %#v", *cfg)

	// Create mutator.
	mt := kwhmutating.MutatorFunc(func(_ context.Context, _ *kwhmodel.AdmissionReview, obj metav1.Object) (*kwhmutating.MutatorResult, error) {
		deployment, ok := obj.(*appsv1.Deployment)
		if !ok {
			return &kwhmutating.MutatorResult{}, nil
		}

		if deployment.Name == "nginx-ingress-controller" && deployment.Namespace == "garden" {
			containers := deployment.Spec.Template.Spec.Containers
			for i, c := range containers {
				if slices.Contains(cfg.mutations, "nginx-ingress-controller") && c.Name == "nginx-ingress-controller" {
					logger.Infof("patching nginx-ingress-controller liveness probe")
					c.LivenessProbe.InitialDelaySeconds = 600

					if strings.Contains(c.Image, "/ingress-nginx/controller-chroot:") && !slices.Contains(c.SecurityContext.Capabilities.Add, "SYS_CHROOT") {
						logger.Infof("patching nginx-ingress-controller with chroot image missing SYS_CHROOT capability")
						c.SecurityContext.Capabilities.Add = append(c.SecurityContext.Capabilities.Add, "SYS_CHROOT")
					}

					deployment.Spec.Template.Spec.Containers[i] = c

					return &kwhmutating.MutatorResult{MutatedObject: deployment}, nil
				}
			}
		} else if slices.Contains(cfg.mutations, "gardenlet") && deployment.Name == "gardenlet" && deployment.Namespace == "garden" {
			logger.Infof("patching gardenlet pod security context")

			deployment.Spec.Template.Spec.SecurityContext = &v1.PodSecurityContext{
				FSGroup: pointer.Pointer(int64(65534)),
			}
		} else if slices.Contains(cfg.mutations, "provider-gcp") && deployment.Name == "gardener-extension-provider-gcp" {
			logger.Infof("removing provider-gcp pod anti affinity rule")

			deployment.Spec.Template.Spec.Affinity.PodAntiAffinity = nil
		}

		logger.Infof("no mutation applied to: %s/%s", deployment.Namespace, deployment.Name)

		return &kwhmutating.MutatorResult{}, nil
	})

	// Create webhook.
	mcfg := kwhmutating.WebhookConfig{
		ID:      "metal-seed-mutator.metal-stack.dev",
		Mutator: mt,
		Logger:  logger,
	}
	wh, err := kwhmutating.NewWebhook(mcfg)
	if err != nil {
		return fmt.Errorf("error creating webhook: %w", err)
	}

	// Get HTTP handler from webhook.
	whHandler, err := kwhhttp.HandlerFor(kwhhttp.HandlerConfig{Webhook: wh, Logger: logger})
	if err != nil {
		return fmt.Errorf("error creating webhook handler: %w", err)
	}

	// Serve.
	logger.Infof("Listening on :8080")
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
