package controller

import (
	"fmt"

	"github.com/appscode/go/types"
	"github.com/aws/aws-sdk-go/aws"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"
	kutil "kmodules.xyz/client-go"
	app_util "kmodules.xyz/client-go/apps/v1"
	core_util "kmodules.xyz/client-go/core/v1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/pkg/eventer"
)

const (
	securityContextCode = int64(65535)
	congigMountPath     = "/etc/config"
	PostgresPort        = 5432
	PostgresPortName    = "api"
)

func (c *Controller) ensureStatefulSet(
	pgbouncer *api.PgBouncer,
	envList []core.EnvVar,
) (kutil.VerbType, error) {

	if err := c.checkStatefulSet(pgbouncer); err != nil {
		return kutil.VerbUnchanged, err
	}
	if err := c.checkConfigMap(pgbouncer); err != nil {
		return kutil.VerbUnchanged, err
	}
	statefulSetMeta := metav1.ObjectMeta{
		Name:      pgbouncer.OffshootName(),
		Namespace: pgbouncer.Namespace,
	}

	ref, rerr := reference.GetReference(clientsetscheme.Scheme, pgbouncer)
	if rerr != nil {
		return kutil.VerbUnchanged, rerr
	}

	replicas := int32(1)
	if pgbouncer.Spec.Replicas != nil {
		replicas = types.Int32(pgbouncer.Spec.Replicas)
	}
	image := "rezoan/pb:latest"
	if pgbouncer.Spec.Image != "" {
		image = pgbouncer.Spec.Image
	}

	statefulSet, vt, err := app_util.CreateOrPatchStatefulSet(c.Client, statefulSetMeta, func(in *apps.StatefulSet) *apps.StatefulSet {
		in.Labels = pgbouncer.OffshootLabels()
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)

		in.Spec.Replicas = types.Int32P(replicas)

		in.Spec.ServiceName = c.GoverningService
		in.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: pgbouncer.OffshootSelectors(),
		}
		in.Spec.Template.Labels = pgbouncer.OffshootSelectors()
		//in.Spec.Template.Spec.InitContainers = core_util.UpsertContainers(in.Spec.Template.Spec.InitContainers, pgbouncer.Spec.PodTemplate.Spec.InitContainers)
		in.Spec.Template.Spec.Containers = core_util.UpsertContainer(
			in.Spec.Template.Spec.Containers,
			core.Container{
				Name: api.ResourceSingularPgBouncer,
				//Args: append([]string{
				//	fmt.Sprintf(`--enable-analytics=%v`, c.EnableAnalytics),
				//}, c.LoggerOptions.ToFlags()...),
				//Env: []core.EnvVar{
				//	{
				//		Name:  analytics.Key,
				//		Value: c.AnalyticsClientID,
				//	},
				//},
				Image:           image,
				ImagePullPolicy: core.PullIfNotPresent,
				SecurityContext: &core.SecurityContext{
					RunAsUser: aws.Int64(securityContextCode),
				},
				VolumeMounts: []core.VolumeMount{
					{
						Name:      pgbouncer.OffshootName(),
						MountPath: congigMountPath,
					},
				},
			})
		in.Spec.Template.Spec.Volumes = []core.Volume{
			{
				Name: pgbouncer.OffshootName(),
				VolumeSource: core.VolumeSource{
					ConfigMap: &core.ConfigMapVolumeSource{
						LocalObjectReference: core.LocalObjectReference{
							Name: pgbouncer.OffshootName(),
						},
					},
				},
			},
		}
		in = upsertPort(in)
		in.Spec.UpdateStrategy = pgbouncer.Spec.UpdateStrategy

		return in
	})

	if err != nil {
		return kutil.VerbUnchanged, err
	}

	if vt == kutil.VerbCreated || vt == kutil.VerbPatched {
		// Check StatefulSet Pod status
		if err := c.CheckStatefulSetPodStatus(statefulSet); err != nil {
			return kutil.VerbUnchanged, err
		}

		c.recorder.Eventf(
			pgbouncer,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully %v StatefulSet",
			vt,
		)
	}

	// ensure pdb
	if err := c.CreateStatefulSetPodDisruptionBudget(statefulSet); err != nil {
		return vt, err
	}
	return vt, nil
}

func (c *Controller) CheckStatefulSetPodStatus(statefulSet *apps.StatefulSet) error {
	err := core_util.WaitUntilPodRunningBySelector(
		c.Client,
		statefulSet.Namespace,
		statefulSet.Spec.Selector,
		int(types.Int32(statefulSet.Spec.Replicas)),
	)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) checkStatefulSet(pgbouncer *api.PgBouncer) error {
	//Name validation for StatefulSet
	// Check whether a non-kubedb managed StatefulSet by this name already exists
	name := pgbouncer.OffshootName()
	// SatatefulSet for PgBouncer database
	statefulSet, err := c.Client.AppsV1().StatefulSets(pgbouncer.Namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		if kerr.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}

	if statefulSet.Labels[api.LabelDatabaseKind] != api.ResourceKindPgBouncer ||
		statefulSet.Labels[api.LabelDatabaseName] != name {
		return fmt.Errorf(`intended statefulSet "%v/%v" already exists`, pgbouncer.Namespace, name)
	}

	return nil
}

func (c *Controller) checkConfigMap(pgbouncer *api.PgBouncer) error {
	//Name validation for configMap
	// Check whether a non-kubedb managed configMap by this name already exists
	name := pgbouncer.OffshootName()
	// configMap for PgBouncer
	configMap, err := c.Client.CoreV1().ConfigMaps(pgbouncer.Namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		if kerr.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}

	if configMap.Labels[api.LabelDatabaseKind] != api.ResourceKindPgBouncer ||
		configMap.Labels[api.LabelDatabaseName] != name {
		return fmt.Errorf(`intended configMap "%v/%v" already exists`, pgbouncer.Namespace, name)
	}

	return nil
}

func upsertPort(statefulSet *apps.StatefulSet) *apps.StatefulSet {
	getPorts := func() []core.ContainerPort {
		portList := []core.ContainerPort{
			{
				Name:          PostgresPortName,
				ContainerPort: PostgresPort,
				Protocol:      core.ProtocolTCP,
			},
		}
		return portList
	}

	for i, container := range statefulSet.Spec.Template.Spec.Containers {
		if container.Name == api.ResourceSingularPgBouncer {
			statefulSet.Spec.Template.Spec.Containers[i].Ports = getPorts()
			return statefulSet
		}
	}

	return statefulSet
}
