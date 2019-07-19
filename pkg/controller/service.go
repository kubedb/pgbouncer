package controller

import (
	"fmt"

	"github.com/appscode/go/log"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"
	kutil "kmodules.xyz/client-go"
	core_util "kmodules.xyz/client-go/core/v1"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	ofst "kmodules.xyz/offshoot-api/api/v1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/pkg/eventer"
)

var (
	NodeRole = "kubedb.com/role"
)

const (
	PgBouncerPort     = 5432
	PgBouncerPortName = "api"
)

func (c *Controller) ensureService(pgbouncer *api.PgBouncer) (kutil.VerbType, error) {
	// Check if service name exists
	err := c.checkService(pgbouncer, pgbouncer.OffshootName())
	if err != nil {
		return kutil.VerbUnchanged, err
	}
	// create database Service
	vt1, err := c.createService(pgbouncer)
	if err != nil {
		return kutil.VerbUnchanged, err
	} else if vt1 != kutil.VerbUnchanged {
		c.recorder.Eventf(
			pgbouncer,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully %s Service",
			vt1,
		)
	}

	// Check if service name exists
	//err = c.checkService(pgbouncer, pgbouncer.ReplicasServiceName())
	//if err != nil {
	//	return kutil.VerbUnchanged, err
	//}
	// create database Service
	//vt2, err := c.createReplicasService(pgbouncer)
	//if err != nil {
	//	return kutil.VerbUnchanged, err
	//} else if vt2 != kutil.VerbUnchanged {
	//	c.recorder.Eventf(
	//		pgbouncer,
	//		core.EventTypeNormal,
	//		eventer.EventReasonSuccessful,
	//		"Successfully %s Service",
	//		vt2,
	//	)
	//}
	//
	//if vt1 == kutil.VerbCreated && vt2 == kutil.VerbCreated {
	//	return kutil.VerbCreated, nil
	//} else if vt1 == kutil.VerbPatched || vt2 == kutil.VerbPatched {
	//	return kutil.VerbPatched, nil
	//}

	return kutil.VerbUnchanged, nil
}

func (c *Controller) checkService(pgbouncer *api.PgBouncer, name string) error {
	service, err := c.Client.CoreV1().Services(pgbouncer.Namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		if kerr.IsNotFound(err) {
			return nil
		}
		return err
	}

	if service.Labels[api.LabelDatabaseKind] != api.ResourceKindPgBouncer ||
		service.Labels[api.LabelDatabaseName] != pgbouncer.Name {
		return fmt.Errorf(`intended service "%v/%v" already exists`, pgbouncer.Namespace, name)
	}

	return nil
}

func (c *Controller) createService(pgbouncer *api.PgBouncer) (kutil.VerbType, error) {
	meta := metav1.ObjectMeta{
		Name:      pgbouncer.OffshootName(),
		Namespace: pgbouncer.Namespace,
	}

	ref, rerr := reference.GetReference(clientsetscheme.Scheme, pgbouncer)
	if rerr != nil {
		return kutil.VerbUnchanged, rerr
	}

	_, ok, err := core_util.CreateOrPatchService(c.Client, meta, func(in *core.Service) *core.Service {
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
		in.Labels = pgbouncer.OffshootLabels()

		in.Spec.Selector = pgbouncer.OffshootSelectors()
		//in.Spec.Selector[NodeRole] = "primary"
		in.Spec.Ports = upsertServicePort(in, pgbouncer)

		if pgbouncer.Spec.ServiceTemplate.Spec.ClusterIP != "" {
			in.Spec.ClusterIP = pgbouncer.Spec.ServiceTemplate.Spec.ClusterIP
		}
		if pgbouncer.Spec.ServiceTemplate.Spec.Type != "" {
			in.Spec.Type = pgbouncer.Spec.ServiceTemplate.Spec.Type
		}
		in.Spec.ExternalIPs = pgbouncer.Spec.ServiceTemplate.Spec.ExternalIPs
		in.Spec.LoadBalancerIP = pgbouncer.Spec.ServiceTemplate.Spec.LoadBalancerIP
		in.Spec.LoadBalancerSourceRanges = pgbouncer.Spec.ServiceTemplate.Spec.LoadBalancerSourceRanges
		in.Spec.ExternalTrafficPolicy = pgbouncer.Spec.ServiceTemplate.Spec.ExternalTrafficPolicy
		if pgbouncer.Spec.ServiceTemplate.Spec.HealthCheckNodePort > 0 {
			in.Spec.HealthCheckNodePort = pgbouncer.Spec.ServiceTemplate.Spec.HealthCheckNodePort
		}
		return in
	})
	return ok, err
}

func upsertServicePort(in *core.Service, pgbouncer *api.PgBouncer) []core.ServicePort {
	var listenPort int32
	if pgbouncer.Spec.ConnectionPoolConfig.ListenPort != nil {

		listenPort = *pgbouncer.Spec.ConnectionPoolConfig.ListenPort
	} else {
		listenPort = PgBouncerPort
	}

	var defaultDBPort = core.ServicePort{
		Name: PgBouncerPortName,
		Port: listenPort,
	}
	return ofst.MergeServicePorts(
		core_util.MergeServicePorts(in.Spec.Ports, []core.ServicePort{defaultDBPort}),
		pgbouncer.Spec.ServiceTemplate.Spec.Ports,
	)
}

//
//func (c *Controller) createReplicasService(pgbouncer *api.PgBouncer) (kutil.VerbType, error) {
//	meta := metav1.ObjectMeta{
//		Name:      pgbouncer.ReplicasServiceName(),
//		Namespace: pgbouncer.Namespace,
//	}
//
//	ref, rerr := reference.GetReference(clientsetscheme.Scheme, pgbouncer)
//	if rerr != nil {
//		return kutil.VerbUnchanged, rerr
//	}
//
//	_, ok, err := core_util.CreateOrPatchService(c.Client, meta, func(in *core.Service) *core.Service {
//		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
//		in.Labels = pgbouncer.OffshootLabels()
//		//in.Annotations = pgbouncer.Spec.ReplicaServiceTemplate.Annotations
//
//		in.Spec.Selector = pgbouncer.OffshootSelectors()
//		in.Spec.Selector[NodeRole] = "replica"
//		in.Spec.Ports = upsertReplicaServicePort(in, pgbouncer)
//
//		//if pgbouncer.Spec.ReplicaServiceTemplate.Spec.ClusterIP != "" {
//		//	in.Spec.ClusterIP = pgbouncer.Spec.ReplicaServiceTemplate.Spec.ClusterIP
//		//}
//		//if pgbouncer.Spec.ReplicaServiceTemplate.Spec.Type != "" {
//		//	in.Spec.Type = pgbouncer.Spec.ReplicaServiceTemplate.Spec.Type
//		//}
//		//in.Spec.ExternalIPs = pgbouncer.Spec.ReplicaServiceTemplate.Spec.ExternalIPs
//		//in.Spec.LoadBalancerIP = pgbouncer.Spec.ReplicaServiceTemplate.Spec.LoadBalancerIP
//		//in.Spec.LoadBalancerSourceRanges = pgbouncer.Spec.ReplicaServiceTemplate.Spec.LoadBalancerSourceRanges
//		//in.Spec.ExternalTrafficPolicy = pgbouncer.Spec.ReplicaServiceTemplate.Spec.ExternalTrafficPolicy
//		//if pgbouncer.Spec.ReplicaServiceTemplate.Spec.HealthCheckNodePort > 0 {
//		//	in.Spec.HealthCheckNodePort = pgbouncer.Spec.ReplicaServiceTemplate.Spec.HealthCheckNodePort
//		//}
//		return in
//	})
//	return ok, err
//}
//
//func upsertReplicaServicePort(in *core.Service, pgbouncer *api.PgBouncer) []core.ServicePort {
//	return ofst.MergeServicePorts(
//		core_util.MergeServicePorts(in.Spec.Ports, []core.ServicePort{defaultDBPort}),
//		pgbouncer.Spec.ReplicaServiceTemplate.Spec.Ports,
//	)
//}

func (c *Controller) ensureStatsService(pgbouncer *api.PgBouncer) (kutil.VerbType, error) {
	// return if monitoring is not prometheus
	if pgbouncer.GetMonitoringVendor() != mona.VendorPrometheus {
		log.Infoln("pgbouncer.spec.monitor.agent is not coreos-operator or builtin.")
		return kutil.VerbUnchanged, nil
	}

	// Check if statsService name exists
	if err := c.checkService(pgbouncer, pgbouncer.StatsService().ServiceName()); err != nil {
		return kutil.VerbUnchanged, err
	}

	ref, rerr := reference.GetReference(clientsetscheme.Scheme, pgbouncer)
	if rerr != nil {
		return kutil.VerbUnchanged, rerr
	}

	// reconcile stats service
	meta := metav1.ObjectMeta{
		Name:      pgbouncer.StatsService().ServiceName(),
		Namespace: pgbouncer.Namespace,
	}
	_, vt, err := core_util.CreateOrPatchService(c.Client, meta, func(in *core.Service) *core.Service {
		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
		in.Labels = pgbouncer.StatsServiceLabels()
		in.Spec.Selector = pgbouncer.OffshootSelectors()
		in.Spec.Ports = core_util.MergeServicePorts(in.Spec.Ports, []core.ServicePort{
			{
				Name:       api.PrometheusExporterPortName,
				Protocol:   core.ProtocolTCP,
				Port:       pgbouncer.Spec.Monitor.Prometheus.Port,
				TargetPort: intstr.FromString(api.PrometheusExporterPortName),
			},
		})
		return in
	})
	if err != nil {
		return kutil.VerbUnchanged, err
	} else if vt != kutil.VerbUnchanged {
		c.recorder.Eventf(
			ref,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully %s stats service",
			vt,
		)
	}
	return vt, nil
}
