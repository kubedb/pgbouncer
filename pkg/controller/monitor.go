package controller

import (
	"fmt"

	"github.com/appscode/go/log"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kutil "kmodules.xyz/client-go"
	core_util "kmodules.xyz/client-go/core/v1"
	meta_util "kmodules.xyz/client-go/meta"
	"kmodules.xyz/monitoring-agent-api/agents"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
)

func (c *Controller) newMonitorController(pgbouncer *api.PgBouncer) (mona.Agent, error) {
	monitorSpec := pgbouncer.Spec.Monitor

	if monitorSpec == nil {
		return nil, fmt.Errorf("MonitorSpec not found in %v", pgbouncer.Spec)
	}

	if monitorSpec.Prometheus != nil {
		return agents.New(monitorSpec.Agent, c.Client, c.ApiExtKubeClient, c.promClient), nil
	}

	return nil, fmt.Errorf("monitoring controller not found for %v", monitorSpec)
}

func (c *Controller) addOrUpdateMonitor(pgbouncer *api.PgBouncer) (kutil.VerbType, error) {
	agent, err := c.newMonitorController(pgbouncer)
	if err != nil {
		log.Infoln("=== newMonitorController err = ", err)
		return kutil.VerbUnchanged, err
	}
	log.Infoln("=== Agent create or update monitor")
	return agent.CreateOrUpdate(pgbouncer.StatsService(), pgbouncer.Spec.Monitor)
}

func (c *Controller) deleteMonitor(pgbouncer *api.PgBouncer) (kutil.VerbType, error) {
	agent, err := c.newMonitorController(pgbouncer)
	if err != nil {
		return kutil.VerbUnchanged, err
	}
	return agent.Delete(pgbouncer.StatsService())
}

func (c *Controller) getOldAgent(pgbouncer *api.PgBouncer) mona.Agent {
	service, err := c.Client.CoreV1().Services(pgbouncer.Namespace).Get(pgbouncer.StatsService().ServiceName(), metav1.GetOptions{})
	if err != nil {
		return nil
	}
	oldAgentType, _ := meta_util.GetStringValue(service.Annotations, mona.KeyAgent)
	return agents.New(mona.AgentType(oldAgentType), c.Client, c.ApiExtKubeClient, c.promClient)
}

func (c *Controller) setNewAgent(pgbouncer *api.PgBouncer) error {
	service, err := c.Client.CoreV1().Services(pgbouncer.Namespace).Get(pgbouncer.StatsService().ServiceName(), metav1.GetOptions{})
	if err != nil {
		return err
	}
	_, _, err = core_util.PatchService(c.Client, service, func(in *core.Service) *core.Service {
		in.Annotations = core_util.UpsertMap(in.Annotations, map[string]string{
			mona.KeyAgent: string(pgbouncer.Spec.Monitor.Agent),
		},
		)
		return in
	})
	return err
}

func (c *Controller) manageMonitor(pgbouncer *api.PgBouncer) error {
	oldAgent := c.getOldAgent(pgbouncer)
	if oldAgent == nil {
		log.Infoln("=== Old agent = nil")
	}
	if pgbouncer.Spec.Monitor != nil {
		if oldAgent != nil &&
			oldAgent.GetType() != pgbouncer.Spec.Monitor.Agent {
			log.Infoln("=== OldAgent != nil and oldAgent.GetType() != pgbouncer.Spec.Monitor.Agent")
			if _, err := oldAgent.Delete(pgbouncer.StatsService()); err != nil {
				log.Errorf("error in deleting Prometheus agent. Reason: %s", err)
			}
			log.Infoln("=== pgbouncer.Spec.Monitor.Agent", pgbouncer.Spec.Monitor.Agent)
			log.Infoln("=== pgbouncer.Spec.Monitor.Agent.Vendor()", pgbouncer.Spec.Monitor.Agent.Vendor())
		}
		log.Infoln("=== Add or update Monitor")
		if _, err := c.addOrUpdateMonitor(pgbouncer); err != nil {
			log.Infoln("=== Add or update Monitor err =>", err)
			return err
		}
		return c.setNewAgent(pgbouncer)
	} else if oldAgent != nil {
		if _, err := oldAgent.Delete(pgbouncer.StatsService()); err != nil {
			log.Errorf("error in deleting Prometheus agent. Reason: %s", err)
		}
	}
	return nil
}
