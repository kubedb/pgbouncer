package framework

import (
	"fmt"
	"time"

	"github.com/appscode/go/crypto/rand"
	"github.com/appscode/go/types"
	. "github.com/onsi/gomega"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"
)

const (
	kindEviction = "Eviction"
	PostgresName = "postgres-for-pgbouncer-test"
	DbAlias      = "postgres"
	DbName       = "postgres"
	pbVersion ="1.9.0-r"
	PgBouncerAdmin = "pgbouncer"
	CmdReload = "RELOAD"
)

func (i *Invocation) PgBouncer() *api.PgBouncer {
	return &api.PgBouncer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rand.WithUniqSuffix(api.ResourceSingularPgBouncer),
			Namespace: i.namespace,
			Labels: map[string]string{
				"app": i.app,
			},
		},
		Spec: api.PgBouncerSpec{
			Version: pbVersion,
			Replicas: types.Int32P(1),
			Databases: []api.Databases{
				{
					Alias:               DbAlias,
					DbName:              DbName,
					AppBindingNamespace: i.namespace,
					AppBindingName:      PostgresName,
				},
			},

			ConnectionPool: &api.ConnectionPoolConfig{
				AdminUsers: []string{
					"admin1",
				},
			},

			SecretList: []api.SecretList{
				{
					SecretName:      PostgresName + "-auth",
					SecretNamespace: i.namespace,
				},
			},

			//TODO: Create a full spec
		},
	}
}

func (f *Framework) CreatePgBouncer(obj *api.PgBouncer) error {
	_, err := f.dbClient.KubedbV1alpha1().PgBouncers(obj.Namespace).Create(obj)
	return err
}

func (f *Framework) GetPgBouncer(meta metav1.ObjectMeta) (*api.PgBouncer, error) {
	return f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
}

func (f *Framework) PatchPgBouncer(meta metav1.ObjectMeta, transform func(pgbouncer *api.PgBouncer) *api.PgBouncer) (*api.PgBouncer, error) {
	pgbouncer, err := f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	pgbouncer, _, err = util.PatchPgBouncer(f.dbClient.KubedbV1alpha1(), pgbouncer, transform)
	return pgbouncer, err
}

func (f *Framework) DeletePgBouncer(meta metav1.ObjectMeta) error {
	return f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Delete(meta.Name, deleteInForeground())
}

func (f *Framework) EventuallyPgBouncer(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			_, err := f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
			if err != nil {
				if kerr.IsNotFound(err) {
					return false
				}
				Expect(err).NotTo(HaveOccurred())
			}
			return true
		},
		time.Minute*5,
		time.Second*5,
	)
}

func (f *Framework) EventuallyPgBouncerPhase(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() api.DatabasePhase {
			db, err := f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			return db.Status.Phase
		},
		time.Minute*5,
		time.Second*5,
	)
}

func (f *Framework) EventuallyPgBouncerPodCount(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() int32 {
			st, err := f.kubeClient.AppsV1beta1().StatefulSets(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
			if err != nil {
				if kerr.IsNotFound(err) {
					return -1
				} else {
					Expect(err).NotTo(HaveOccurred())
				}
			}
			return st.Status.ReadyReplicas
		},
		time.Minute*5,
		time.Second*5,
	)
}

func (f *Framework) EventuallyPgBouncerRunning(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			pgbouncer, err := f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			return pgbouncer.Status.Phase == api.DatabasePhaseRunning
		},
		time.Minute*15,
		time.Second*5,
	)
}

func (f *Framework) CleanPgBouncer() {
	pgbouncerList, err := f.dbClient.KubedbV1alpha1().PgBouncers(f.namespace).List(metav1.ListOptions{})
	if err != nil {
		return
	}
	for _, e := range pgbouncerList.Items {
		if _, _, err := util.PatchPgBouncer(f.dbClient.KubedbV1alpha1(), &e, func(in *api.PgBouncer) *api.PgBouncer {
			in.ObjectMeta.Finalizers = nil
			//in.Spec.TerminationPolicy = api.TerminationPolicyWipeOut
			return in
		}); err != nil {
			fmt.Printf("error Patching PgBouncer. error: %v", err)
		}
	}
	if err := f.dbClient.KubedbV1alpha1().PgBouncers(f.namespace).DeleteCollection(deleteInForeground(), metav1.ListOptions{}); err != nil {
		fmt.Printf("error in deletion of PgBouncer. Error: %v", err)
	}
}

//func (f *Framework) EvictPodsFromStatefulSet(meta metav1.ObjectMeta) error {
//	var err error
//	labelSelector := labels.Set{
//		meta_util.ManagedByLabelKey: api.GenericKey,
//		api.LabelDatabaseKind:       api.ResourceKindPgBouncer,
//		api.LabelDatabaseName:       meta.GetName(),
//	}
//	// get sts in the namespace
//	stsList, err := f.kubeClient.AppsV1().StatefulSets(meta.Namespace).List(metav1.ListOptions{LabelSelector: labelSelector.String()})
//	if err != nil {
//		return err
//	}
//	for _, sts := range stsList.Items {
//		// if PDB is not found, send error
//		var pdb *policy.PodDisruptionBudget
//		pdb, err = f.kubeClient.PolicyV1beta1().PodDisruptionBudgets(sts.Namespace).Get(sts.Name, metav1.GetOptions{})
//		if err != nil {
//			return err
//		}
//		eviction := &policy.Eviction{
//			TypeMeta: metav1.TypeMeta{
//				APIVersion: policy.SchemeGroupVersion.String(),
//				Kind:       kindEviction,
//			},
//			ObjectMeta: metav1.ObjectMeta{
//				Name:      sts.Name,
//				Namespace: sts.Namespace,
//			},
//			DeleteOptions: &metav1.DeleteOptions{},
//		}
//
//		if pdb.Spec.MaxUnavailable == nil {
//			return fmt.Errorf("found pdb %s spec.maxUnavailable nil", pdb.Name)
//		}
//
//		// try to evict as many pod as allowed in pdb. No err should occur
//		maxUnavailable := pdb.Spec.MaxUnavailable.IntValue()
//		for i := 0; i < maxUnavailable; i++ {
//			eviction.Name = sts.Name + "-" + strconv.Itoa(i)
//
//			err := f.kubeClient.PolicyV1beta1().Evictions(eviction.Namespace).Evict(eviction)
//			if err != nil {
//				return err
//			}
//		}
//
//		// try to evict one extra pod. TooManyRequests err should occur
//		eviction.Name = sts.Name + "-" + strconv.Itoa(maxUnavailable)
//		err = f.kubeClient.PolicyV1beta1().Evictions(eviction.Namespace).Evict(eviction)
//		if kerr.IsTooManyRequests(err) {
//			err = nil
//		} else if err != nil {
//			return err
//		} else {
//			return fmt.Errorf("expected pod %s/%s to be not evicted due to pdb %s", sts.Namespace, eviction.Name, pdb.Name)
//		}
//	}
//	return err
//}

func (f *Framework) EventuallyWipedOut(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() error {
			labelMap := map[string]string{
				api.LabelDatabaseName: meta.Name,
				api.LabelDatabaseKind: api.ResourceKindPostgres,
			}
			labelSelector := labels.SelectorFromSet(labelMap)

			// check if pvcs is wiped out
			pvcList, err := f.kubeClient.CoreV1().PersistentVolumeClaims(meta.Namespace).List(
				metav1.ListOptions{
					LabelSelector: labelSelector.String(),
				},
			)
			if err != nil {
				return err
			}
			if len(pvcList.Items) > 0 {
				fmt.Println("PVCs have not wiped out yet")
				return fmt.Errorf("PVCs have not wiped out yet")
			}

			// check if snapshot is wiped out
			snapshotList, err := f.dbClient.KubedbV1alpha1().Snapshots(meta.Namespace).List(
				metav1.ListOptions{
					LabelSelector: labelSelector.String(),
				},
			)
			if err != nil {
				return err
			}
			if len(snapshotList.Items) > 0 {
				fmt.Println("all snapshots have not wiped out yet")
				return fmt.Errorf("all snapshots have not wiped out yet")
			}

			// check if secrets are wiped out
			secretList, err := f.kubeClient.CoreV1().Secrets(meta.Namespace).List(
				metav1.ListOptions{
					LabelSelector: labelSelector.String(),
				},
			)
			if err != nil {
				return err
			}
			if len(secretList.Items) > 0 {
				fmt.Println("secrets have not wiped out yet")
				return fmt.Errorf("secrets have not wiped out yet")
			}

			// check if appbinds are wiped out
			appBindingList, err := f.appCatalogClient.AppBindings(meta.Namespace).List(
				metav1.ListOptions{
					LabelSelector: labelSelector.String(),
				},
			)
			if err != nil {
				return err
			}
			if len(appBindingList.Items) > 0 {
				fmt.Println("appBindings have not wiped out yet")
				return fmt.Errorf("appBindings have not wiped out yet")
			}

			return nil
		},
		time.Minute*10,
		time.Second*5,
	)
}
