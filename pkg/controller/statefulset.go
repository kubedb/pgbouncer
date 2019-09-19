package controller

import (
	"fmt"

	"github.com/appscode/go/log"
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
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	catalog "kubedb.dev/apimachinery/apis/catalog/v1alpha1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/pkg/eventer"
)

const (
	securityContextCode = int64(65535)
	configMountPath     = "/etc/config"
	userListMountPath   = "/var/run/pgbouncer/secrets"
)

func (c *Controller) ensureStatefulSet(
	pgbouncer *api.PgBouncer,
	pgbouncerVersion *catalog.PgBouncerVersion,
	envList []core.EnvVar,
) (kutil.VerbType, error) {
	if err := c.checkConfigMap(pgbouncer); err != nil {
		if kerr.IsNotFound(err) {
			_, _ = c.ensureConfigMapFromCRD(pgbouncer)

		} else {
			return kutil.VerbUnchanged, err
		}
	}
	if err := c.checkStatefulSet(pgbouncer); err != nil {
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
	image := pgbouncerVersion.Spec.Server.Image

	statefulSet, vt, err := app_util.CreateOrPatchStatefulSet(c.Client, statefulSetMeta, func(in *apps.StatefulSet) *apps.StatefulSet {
		in.Annotations = pgbouncer.Annotations //TODO: actual annotations
		in.Labels = pgbouncer.OffshootLabels()
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)

		in.Spec.Replicas = types.Int32P(replicas)

		in.Spec.ServiceName = c.GoverningService
		in.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: pgbouncer.OffshootSelectors(),
		}
		in.Spec.Template.Labels = pgbouncer.OffshootSelectors()

		var volumes []core.Volume
		configMapVolume := core.Volume{
			Name: pgbouncer.OffshootName(),
			VolumeSource: core.VolumeSource{
				ConfigMap: &core.ConfigMapVolumeSource{
					LocalObjectReference: core.LocalObjectReference{
						Name: pgbouncer.OffshootName(),
					},
				},
			},
		}
		volumes = append(volumes, configMapVolume)

		var volumeMounts []core.VolumeMount
		configMapVolumeMount := core.VolumeMount{
			Name:      pgbouncer.OffshootName(),
			MountPath: configMountPath,
		}
		volumeMounts = append(volumeMounts, configMapVolumeMount)

		if pgbouncer.Spec.UserList.SecretName != "" { //Add secret (user list file) as volume
			secretVolume, secretVolumeMount, err := c.getVolumeAndVoulumeMountForUserList(pgbouncer)
			if err == nil {
				volumes = append(volumes, *secretVolume)
				//Add to volumeMounts to mount the volume
				volumeMounts = append(volumeMounts, *secretVolumeMount)
			} else if kerr.IsNotFound(err) {
				log.Infoln("UserList secret " + pgbouncer.Spec.UserList.SecretNamespace + "/" + pgbouncer.Spec.UserList.SecretName + " is not available")
			}
			//We are not concerned about other errors
		}
		in.Spec.Template.Spec.InitContainers = core_util.UpsertContainers(in.Spec.Template.Spec.InitContainers, pgbouncer.Spec.PodTemplate.Spec.InitContainers)
		in.Spec.Template.Spec.Containers = core_util.UpsertContainer(
			in.Spec.Template.Spec.Containers,
			core.Container{
				Name: api.ResourceSingularPgBouncer,
				//TODO: decide what to do with Args and Env
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
				VolumeMounts: volumeMounts,

				Resources:      pgbouncer.Spec.PodTemplate.Spec.Resources,
				LivenessProbe:  pgbouncer.Spec.PodTemplate.Spec.LivenessProbe,
				ReadinessProbe: pgbouncer.Spec.PodTemplate.Spec.ReadinessProbe,
				Lifecycle:      pgbouncer.Spec.PodTemplate.Spec.Lifecycle,
			})

		in.Spec.Template.Spec.Volumes = volumes
		in = upsertUserEnv(in, pgbouncer)
		in = upsertPort(in, pgbouncer)
		in.Spec.Template.Spec.NodeSelector = pgbouncer.Spec.PodTemplate.Spec.NodeSelector
		in.Spec.Template.Spec.Affinity = pgbouncer.Spec.PodTemplate.Spec.Affinity
		in.Spec.Template.Spec.Tolerations = pgbouncer.Spec.PodTemplate.Spec.Tolerations
		in.Spec.Template.Spec.ImagePullSecrets = pgbouncer.Spec.PodTemplate.Spec.ImagePullSecrets
		in.Spec.Template.Spec.PriorityClassName = pgbouncer.Spec.PodTemplate.Spec.PriorityClassName
		in.Spec.Template.Spec.Priority = pgbouncer.Spec.PodTemplate.Spec.Priority
		in.Spec.Template.Spec.SecurityContext = pgbouncer.Spec.PodTemplate.Spec.SecurityContext
		in = c.upsertMonitoringContainer(in, pgbouncer, pgbouncerVersion)

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

// upsertUserEnv add/overwrite env from user provided env in crd spec
func upsertUserEnv(statefulSet *apps.StatefulSet, pgbouncer *api.PgBouncer) *apps.StatefulSet {
	for i, container := range statefulSet.Spec.Template.Spec.Containers {
		if container.Name == api.ResourceSingularPgBouncer {
			statefulSet.Spec.Template.Spec.Containers[i].Env = core_util.UpsertEnvVars(container.Env, pgbouncer.Spec.PodTemplate.Spec.Env...)
			return statefulSet
		}
	}
	return statefulSet
}

func upsertPort(statefulSet *apps.StatefulSet, pgbouncer *api.PgBouncer) *apps.StatefulSet {
	getPorts := func() []core.ContainerPort {
		portList := []core.ContainerPort{
			{
				Name:          PgBouncerPortName,
				ContainerPort: *pgbouncer.Spec.ConnectionPool.ListenPort,
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

func (c *Controller) upsertMonitoringContainer(statefulSet *apps.StatefulSet, pgbouncer *api.PgBouncer, pgbouncerVersion *catalog.PgBouncerVersion) *apps.StatefulSet {
	if pgbouncer.GetMonitoringVendor() == mona.VendorPrometheus {
		var monitorArgs []string
		if pgbouncer.Spec.Monitor != nil {
			monitorArgs = pgbouncer.Spec.Monitor.Args
		}
		container := core.Container{
			Name: "exporter",
			//TODO: decide what to do with Args
			Args: append([]string{
				fmt.Sprintf("--web.listen-address=:%d",api.PrometheusExporterPortNumber),
			}, monitorArgs...),
			Image:           pgbouncerVersion.Spec.Exporter.Image,
			ImagePullPolicy: core.PullIfNotPresent,
			Ports: []core.ContainerPort{
				{
					Name:          api.PrometheusExporterPortName,
					Protocol:      core.ProtocolTCP,
					ContainerPort: int32(api.PrometheusExporterPortNumber),
				},
			},
			Env:             pgbouncer.Spec.Monitor.Env,
			Resources:       pgbouncer.Spec.Monitor.Resources,
			SecurityContext: pgbouncer.Spec.Monitor.SecurityContext,
		}

		envList := []core.EnvVar{
			{
				Name:  "DATA_SOURCE_NAME",
				Value: fmt.Sprintf("postgres://pgbouncer:@localhost:%d?sslmode=disable", *pgbouncer.Spec.ConnectionPool.ListenPort),
			},
			{
				Name:  "PGPASSWORD",
				Value: "kubedb",
			},
		}

		container.Env = core_util.UpsertEnvVars(container.Env, envList...)
		containers := statefulSet.Spec.Template.Spec.Containers
		containers = core_util.UpsertContainer(containers, container)
		statefulSet.Spec.Template.Spec.Containers = containers
	}
	return statefulSet
}
