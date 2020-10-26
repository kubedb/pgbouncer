/*
Copyright AppsCode Inc. and Contributors

Licensed under the AppsCode Free Trial License 1.0.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://github.com/appscode/licenses/raw/1.0.0/AppsCode-Free-Trial-1.0.0.md

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha2"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha2/util"
	"kubedb.dev/apimachinery/pkg/eventer"
	validator "kubedb.dev/pgbouncer/pkg/admission"

	"github.com/appscode/go/log"
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kutil "kmodules.xyz/client-go"
	dynamic_util "kmodules.xyz/client-go/dynamic"
)

func (c *Controller) runPgBouncer(key string) error {
	log.Debugln("started processing, key:", key)
	obj, exists, err := c.pbInformer.GetIndexer().GetByKey(key)
	if err != nil {
		log.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		log.Infof("PgBouncer %s does not exist anymore\n", key)
		log.Debugf("PgBouncer %s does not exist anymore", key)
	} else {
		// Note that you also have to check the uid if you have a local controlled resource, which
		// is dependent on the actual instance, to detect that a PgBouncer was recreated with the same name
		pgbouncer := obj.(*api.PgBouncer).DeepCopy()

		if err := c.syncPgBouncer(pgbouncer); err != nil {
			log.Errorln(err)
			c.pushFailureEvent(pgbouncer, err.Error())
			return err
		}
	}
	return nil
}

func (c *Controller) syncPgBouncer(db *api.PgBouncer) error {
	if err := c.manageValidation(db); err != nil {
		log.Infoln(err)
		return nil // user err, dont' retry.
	}
	if err := c.manageInitialPhase(db); err != nil {
		log.Infoln(err)
		return err
	}
	// ensure Governing Service
	if err := c.ensureGoverningService(db); err != nil {
		return fmt.Errorf(`failed to create governing Service for : "%v/%v". Reason: %v`, db.Namespace, db.Name, err)
	}
	// create or patch Service
	if err := c.ensureService(db); err != nil {
		log.Infoln(err)
		return err
	}
	// create or patch default Secret
	if err := c.syncAuthSecret(db); err != nil {
		log.Infoln(err)
		return err
	}
	// create or patch Secret
	if err := c.manageSecret(db); err != nil {
		log.Infoln(err)
		return err
	}
	// wait for certificates
	if db.Spec.TLS != nil {
		ok, err := dynamic_util.ResourcesExists(
			c.DynamicClient,
			core.SchemeGroupVersion.WithResource("secrets"),
			db.Namespace,
			db.MustCertSecretName(api.PgBouncerServerCert),
			db.MustCertSecretName(api.PgBouncerClientCert),
			db.MustCertSecretName(api.PgBouncerMetricsExporterCert),
		)
		if err != nil {
			return err
		}
		if !ok {
			log.Infof("wait for all certificate secrets for pgbouncer %s/%s", db.Namespace, db.Name)
			return nil
		}
	}
	// create or patch StatefulSet
	if err := c.manageStatefulSet(db); err != nil {
		log.Infoln(err)
		return err
	}
	// create or patch Stat service
	if err := c.syncStatService(db); err != nil {
		log.Infoln(err)
		return err
	}

	if err := c.manageMonitor(db); err != nil {
		c.recorder.Eventf(
			db,
			core.EventTypeWarning,
			eventer.EventReasonFailedToCreate,
			"Failed to manage monitoring system. Reason: %v",
			err,
		)
		log.Errorln(err)
		return nil
	}

	// Add initialized or running phase
	if err := c.manageFinalPhase(db); err != nil {
		log.Infoln(err)
		return err
	}
	return nil
}

func (c *Controller) manageValidation(db *api.PgBouncer) error {
	if err := validator.ValidatePgBouncer(c.Client, c.DBClient, db, true); err != nil {
		c.recorder.Event(
			db,
			core.EventTypeWarning,
			eventer.EventReasonInvalid,
			err.Error(),
		)
		log.Errorln(err)
		return err // user error so just record error and don't retry.
	}

	// Check if userList is absent.
	if db.Spec.UserListSecretRef != nil && db.Spec.UserListSecretRef.Name != "" {
		if db.Spec.ConnectionPool != nil && db.Spec.ConnectionPool.AuthType != "any" {
			if _, err := c.Client.CoreV1().Secrets(db.GetNamespace()).Get(context.TODO(), db.Spec.UserListSecretRef.Name, metav1.GetOptions{}); err != nil {
				c.recorder.Eventf(
					db,
					core.EventTypeWarning,
					"UserListMissing",
					"user-list secret %s not found", db.Spec.UserListSecretRef.Name)
			}
		}
	}

	return nil
}

func (c *Controller) manageInitialPhase(db *api.PgBouncer) error {
	if db.Status.Phase == "" {
		pg, err := util.UpdatePgBouncerStatus(context.TODO(), c.DBClient.KubedbV1alpha2(), db.ObjectMeta, func(in *api.PgBouncerStatus) *api.PgBouncerStatus {
			in.Phase = api.DatabasePhaseProvisioning
			return in
		}, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		db.Status = pg.Status
	}
	return nil
}

func (c *Controller) manageFinalPhase(db *api.PgBouncer) error {
	if !c.PgBouncerExists(db) {
		return nil
	}

	pg, err := util.UpdatePgBouncerStatus(context.TODO(), c.DBClient.KubedbV1alpha2(), db.ObjectMeta, func(in *api.PgBouncerStatus) *api.PgBouncerStatus {
		in.Phase = api.DatabasePhaseReady
		in.ObservedGeneration = db.Generation
		return in
	}, metav1.UpdateOptions{})
	if err != nil {
		log.Infoln(err)
		return err
	}
	db.Status = pg.Status
	return nil
}

func (c *Controller) syncAuthSecret(db *api.PgBouncer) error {
	sVerb, err := c.ensureAuthSecret(db)
	if err != nil {
		return err
	}

	if sVerb == kutil.VerbCreated {
		c.recorder.Event(
			db,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully created PgBouncer Fallback Secret",
		)
	} else if sVerb == kutil.VerbPatched {
		c.recorder.Event(
			db,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully patched PgBouncer Fallback Secret",
		)
	}
	if sVerb != kutil.VerbUnchanged {
		log.Infoln("Default secret ", sVerb)
	}
	return nil
}

func (c *Controller) manageSecret(db *api.PgBouncer) error {
	secretVerb, err := c.ensureConfigSecret(db)
	if err != nil {
		log.Infoln(err)
		return err
	}

	if secretVerb == kutil.VerbCreated {
		c.recorder.Event(
			db,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully created PgBouncer secret",
		)
	} else if secretVerb == kutil.VerbPatched {
		c.recorder.Event(
			db,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully patched PgBouncer secret",
		)
	}
	if secretVerb != kutil.VerbUnchanged {
		log.Infoln("Secret ", secretVerb)
	}

	return nil
}

func (c *Controller) manageStatefulSet(db *api.PgBouncer) error {
	pgBouncerVersion, err := c.DBClient.CatalogV1alpha1().PgBouncerVersions().Get(context.TODO(), db.Spec.Version, metav1.GetOptions{})
	if err != nil {
		log.Infoln(err)
		return err
	}

	statefulSetVerb, err := c.ensureStatefulSet(db, pgBouncerVersion, []core.EnvVar{})
	if err != nil {
		log.Infoln(err)
		return err
	}
	if statefulSetVerb == kutil.VerbCreated {
		c.recorder.Event(
			db,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully created PgBouncer statefulset",
		)
	} else if statefulSetVerb == kutil.VerbPatched {
		c.recorder.Event(
			db,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully patched PgBouncer statefulset",
		)
	}
	if statefulSetVerb != kutil.VerbUnchanged {
		log.Infoln("Statefulset ", statefulSetVerb)
	}
	return nil
}

func (c *Controller) ensureService(db *api.PgBouncer) error {
	vt, err := c.ensurePrimaryService(db)
	if err != nil {
		return err
	}
	if vt != kutil.VerbUnchanged {
		c.recorder.Eventf(
			db,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully %s Service",
			vt,
		)
	}
	if vt != kutil.VerbUnchanged {
		log.Infoln("Service ", vt)
	}
	return nil
}

func (c *Controller) syncStatService(db *api.PgBouncer) error {
	statServiceVerb, err := c.ensureStatsService(db)
	if err != nil {
		return err
	}
	if statServiceVerb == kutil.VerbCreated {
		c.recorder.Event(
			db,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully created Stat Service",
		)
	} else if statServiceVerb == kutil.VerbPatched {
		c.recorder.Event(
			db,
			core.EventTypeNormal,
			eventer.EventReasonSuccessful,
			"Successfully patched Stat Service",
		)
	}
	if statServiceVerb != kutil.VerbUnchanged {
		log.Infoln("Stat Service ", statServiceVerb)
	}
	return nil
}

func (c *Controller) PgBouncerExists(db *api.PgBouncer) bool {
	_, err := c.DBClient.KubedbV1alpha2().PgBouncers(db.Namespace).Get(context.TODO(), db.Name, metav1.GetOptions{})
	return err == nil
}
