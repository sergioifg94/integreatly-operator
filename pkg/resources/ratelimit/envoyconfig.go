package ratelimit

import (
	"context"

	marin3rv1alpha1 "github.com/3scale/marin3r/pkg/apis/marin3r/v1alpha1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func DeleteEnvoyConfigsInNamespaces(ctx context.Context, client k8sclient.Client, namespaces ...string) error {
	for _, namespace := range namespaces {
		envoyConfigs := &marin3rv1alpha1.EnvoyConfigList{}
		if err := client.List(ctx, envoyConfigs, k8sclient.InNamespace(namespace)); err != nil {
			return err
		}

		for _, envoyConfig := range envoyConfigs.Items {
			if err := k8sclient.IgnoreNotFound(
				client.Delete(ctx, &envoyConfig),
			); err != nil {
				return err
			}
		}
	}

	return nil
}
