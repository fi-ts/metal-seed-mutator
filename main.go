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

	kwhhttp "github.com/slok/kubewebhook/v2/pkg/http"
	kwhmodel "github.com/slok/kubewebhook/v2/pkg/model"
	kwhmutating "github.com/slok/kubewebhook/v2/pkg/webhook/mutating"
	"golang.org/x/exp/slices"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

		for _, m := range cfg.mutations {
			if !slices.Contains(allMutations(), m) {
				return nil, fmt.Errorf("unknown mutation: %s", m)
			}
		}
	}

	return cfg, nil
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error running app: %s", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	cfg, err := initFlags()
	if err != nil {
		return err
	}

	wh, err := kwhmutating.NewWebhook(kwhmutating.WebhookConfig{
		ID: "metal-seed-mutator.metal-stack.dev",
		Mutator: kwhmutating.MutatorFunc(func(_ context.Context, _ *kwhmodel.AdmissionReview, obj metav1.Object) (*kwhmutating.MutatorResult, error) {
			mutator := mutator{
				log: logger.With("namespace", obj.GetNamespace(), "name", obj.GetName()),
				cfg: cfg,
			}

			switch o := obj.(type) {
			case *appsv1.Deployment:
				mutator.log = mutator.log.With("kind", "deployment")
				return mutator.mutateDeployment(o)
			case *appsv1.StatefulSet:
				mutator.log = mutator.log.With("kind", "statefulset")
				return mutator.mutateStatefulSet(o)
			default:
				return &kwhmutating.MutatorResult{}, nil
			}
		}),
	})
	if err != nil {
		return fmt.Errorf("error creating webhook: %w", err)
	}

	whHandler, err := kwhhttp.HandlerFor(kwhhttp.HandlerConfig{Webhook: wh})
	if err != nil {
		return fmt.Errorf("error creating webhook handler: %w", err)
	}

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
