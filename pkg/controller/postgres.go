package controller

import (
	"fmt"
	"github.com/appscode/go/encoding/json/types"

	"github.com/appscode/go/log"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"
	kutil "kmodules.xyz/client-go"
	core_util "kmodules.xyz/client-go/core/v1"
	dynamic_util "kmodules.xyz/client-go/dynamic"
	meta_util "kmodules.xyz/client-go/meta"
	"kubedb.dev/apimachinery/apis"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
	"kubedb.dev/apimachinery/pkg/eventer"
	validator "kubedb.dev/pgbouncer/pkg/admission"
)

func (c *Controller) create(pgbouncer *api.PgBouncer) error {
	if err := validator.ValidatePgBouncer(c.Client, c.ExtClient, pgbouncer, true); err != nil {
		c.recorder.Event(
			pgbouncer,
			core.EventTypeWarning,
			eventer.EventReasonInvalid,
			err.Error(),
		)
		log.Errorln(err)
		// stop Scheduler in case there is any.
		return nil // user error so just record error and don't retry.
	}
	println(":::::::::::::::CREATE PGBOUNCER::::::::::::",pgbouncer.Name,"::::::::::::::::")

	if pgbouncer.Status.Phase == "" {
		pg, err := util.UpdatePgBouncerStatus(c.ExtClient.KubedbV1alpha1(), pgbouncer, func(in *api.PgBouncerStatus) *api.PgBouncerStatus {
			in.Phase = api.DatabasePhaseCreating
			return in
		}, apis.EnableStatusSubresource)
		if err != nil {
			return err
		}
		pgbouncer.Status = pg.Status
	}

	// create Governing Service
	governingService := c.GoverningService
	if err := c.CreateGoverningService(governingService, pgbouncer.Namespace); err != nil {
		return fmt.Errorf(`failed to create Service: "%v/%v". Reason: %v`, pgbouncer.Namespace, governingService, err)
	}

	// ensure database Service
	//_, err := c.Client.AppsV1().StatefulSets(pgbouncer.Namespace).Get(pgbouncer.OffshootName(), metav1.GetOptions{})
	configMapVerb, err := c.ensureConfigMapFromCRD(pgbouncer)
	if err != nil {
		return err
	}

	if configMapVerb == kutil.VerbCreated {
		c.recorder.Event(
			pgbouncer,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully created PgBouncer configMap",
		)
	} else if configMapVerb == kutil.VerbPatched {
		c.recorder.Event(
			pgbouncer,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully patched PgBouncer configMap",
		)
	}

	log.Infoln("ConfigMap ", configMapVerb)

	statefulsetVerb, err := c.ensureStatefulSet(pgbouncer, []core.EnvVar{})
	if err != nil {
		return err
	}
	if statefulsetVerb == kutil.VerbCreated {
		c.recorder.Event(
			pgbouncer,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully created PgBouncer statefulset",
		)
	} else if statefulsetVerb == kutil.VerbPatched {
		c.recorder.Event(
			pgbouncer,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully patched PgBouncer statefulset",
		)
	}
	log.Infoln("Statefulset ", statefulsetVerb)

	serviceVerb, err := c.ensureService(pgbouncer)
	if err != nil {
		return err
	}
	if serviceVerb == kutil.VerbCreated {
		c.recorder.Event(
			pgbouncer,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully created Service",
		)
	} else if serviceVerb == kutil.VerbPatched {
		c.recorder.Event(
			pgbouncer,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully patched Service",
		)
	}
	log.Infoln("Service ", serviceVerb)

	if _, err := meta_util.GetString(pgbouncer.Annotations, api.AnnotationInitialized); err == kutil.ErrNotFound {

		println(":::::::::::::Not found err for phase")
		if pgbouncer.Status.Phase == api.DatabasePhaseInitializing {
			println(":::::::Current phase = ", pgbouncer.Status.Phase )
			return nil
		}
		println("::::::::::::Adding phase: Initializing")
		// add phase that database is being initialized
		pg, err := util.UpdatePgBouncerStatus(c.ExtClient.KubedbV1alpha1(), pgbouncer, func(in *api.PgBouncerStatus) *api.PgBouncerStatus {
			in.Phase = api.DatabasePhaseInitializing
			return in
		}, apis.EnableStatusSubresource)
		if err != nil {
			return err
		}
		pgbouncer.Status = pg.Status
	}
	println("::::::::::::Adding phase: Running")
	pg, err := util.UpdatePgBouncerStatus(c.ExtClient.KubedbV1alpha1(), pgbouncer, func(in *api.PgBouncerStatus) *api.PgBouncerStatus {
		in.Phase = api.DatabasePhaseRunning
		in.ObservedGeneration = types.NewIntHash(pgbouncer.Generation, meta_util.GenerationHash(pgbouncer))
		return in
	}, apis.EnableStatusSubresource)
	if err != nil {
		return err
	}
	pgbouncer.Status = pg.Status

	return nil
}

func (c *Controller) terminate(pgbouncer *api.PgBouncer) error {
	ref, rerr := reference.GetReference(clientsetscheme.Scheme, pgbouncer)
	if rerr != nil {
		return rerr
	}
	//FOR any termination policy, delete eveything
	// If TerminationPolicy is "pause", keep everything (ie, PVCs,Secrets,Snapshots) intact.
	if pgbouncer.Spec.TerminationPolicy == api.TerminationPolicyPause {
		if err := c.removeOwnerReferenceFromOffshoots(pgbouncer, ref); err != nil {
			return err
		}

	} else {
		// If TerminationPolicy is "wipeOut", delete everything (ie, PVCs,Secrets,Snapshots,WAL-data).
		// If TerminationPolicy is "delete", delete PVCs and keep snapshots,secrets, wal-data intact.
		// In both these cases, don't create dormantdatabase
		if err := c.setOwnerReferenceToOffshoots(pgbouncer, ref); err != nil {
			return err
		}
	}

	if pgbouncer.Spec.Monitor != nil {
		if _, err := c.deleteMonitor(pgbouncer); err != nil {
			log.Errorln(err)
			return nil
		}
	}
	return nil
}

func (c *Controller) setOwnerReferenceToOffshoots(pgbouncer *api.PgBouncer, ref *core.ObjectReference) error {
	selector := labels.SelectorFromSet(pgbouncer.OffshootSelectors())

	// If TerminationPolicy is "wipeOut", delete snapshots and secrets,
	// else, keep it intact.
	if pgbouncer.Spec.TerminationPolicy == api.TerminationPolicyWipeOut {
		if err := dynamic_util.EnsureOwnerReferenceForSelector(
			c.DynamicClient,
			api.SchemeGroupVersion.WithResource(api.ResourcePluralSnapshot),
			pgbouncer.Namespace,
			selector,
			ref); err != nil {
			return err
		}
		// if wal archiver was configured, remove wal data from backend
	} else {
		// Make sure snapshot and secret's ownerreference is removed.
		if err := dynamic_util.RemoveOwnerReferenceForSelector(
			c.DynamicClient,
			api.SchemeGroupVersion.WithResource(api.ResourcePluralSnapshot),
			pgbouncer.Namespace,
			selector,
			ref); err != nil {
			return err
		}
		if err := dynamic_util.RemoveOwnerReferenceForItems(
			c.DynamicClient,
			core.SchemeGroupVersion.WithResource("secrets"),
			pgbouncer.Namespace,
			nil,
			ref); err != nil {
			return err
		}
	}
	// delete PVC for both "wipeOut" and "delete" TerminationPolicy.
	return dynamic_util.EnsureOwnerReferenceForSelector(
		c.DynamicClient,
		core.SchemeGroupVersion.WithResource("persistentvolumeclaims"),
		pgbouncer.Namespace,
		selector,
		ref)
}

func (c *Controller) removeOwnerReferenceFromOffshoots(pgbouncer *api.PgBouncer, ref *core.ObjectReference) error {
	// First, Get LabelSelector for Other Components
	labelSelector := labels.SelectorFromSet(pgbouncer.OffshootSelectors())

	if err := dynamic_util.RemoveOwnerReferenceForSelector(
		c.DynamicClient,
		api.SchemeGroupVersion.WithResource(api.ResourcePluralSnapshot),
		pgbouncer.Namespace,
		labelSelector,
		ref); err != nil {
		return err
	}
	if err := dynamic_util.RemoveOwnerReferenceForSelector(
		c.DynamicClient,
		core.SchemeGroupVersion.WithResource("persistentvolumeclaims"),
		pgbouncer.Namespace,
		labelSelector,
		ref); err != nil {
		return err
	}
	if err := dynamic_util.RemoveOwnerReferenceForItems(
		c.DynamicClient,
		core.SchemeGroupVersion.WithResource("secrets"),
		pgbouncer.Namespace,
		nil,
		ref); err != nil {
		return err
	}
	return nil
}

func (c *Controller) GetDatabase(meta metav1.ObjectMeta) (runtime.Object, error) {
	pgbouncer, err := c.ExtClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return pgbouncer, nil
}

func (c *Controller) SetDatabaseStatus(meta metav1.ObjectMeta, phase api.DatabasePhase, reason string) error {
	pgbouncer, err := c.ExtClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	_, err = util.UpdatePgBouncerStatus(c.ExtClient.KubedbV1alpha1(), pgbouncer, func(in *api.PgBouncerStatus) *api.PgBouncerStatus {
		in.Phase = phase
		in.Reason = reason
		return in
	}, apis.EnableStatusSubresource)
	return err
}

func (c *Controller) UpsertDatabaseAnnotation(meta metav1.ObjectMeta, annotation map[string]string) error {
	pgbouncer, err := c.ExtClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	_, _, err = util.PatchPgBouncer(c.ExtClient.KubedbV1alpha1(), pgbouncer, func(in *api.PgBouncer) *api.PgBouncer {
		in.Annotations = core_util.UpsertMap(in.Annotations, annotation)
		return in
	})
	return err
}
