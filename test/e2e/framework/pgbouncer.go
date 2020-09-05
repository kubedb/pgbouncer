/*
Copyright AppsCode Inc. and Contributors

Licensed under the AppsCode Community License 1.0.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://github.com/appscode/licenses/raw/1.0.0/AppsCode-Community-1.0.0.md

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package framework

import (
	"context"
	"fmt"
	"time"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/apimachinery/client/clientset/versioned/typed/kubedb/v1alpha1/util"

	"github.com/appscode/go/crypto/rand"
	"github.com/appscode/go/types"
	. "github.com/onsi/gomega"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	kutil "kmodules.xyz/client-go"
	core_util "kmodules.xyz/client-go/core/v1"
	meta_util "kmodules.xyz/client-go/meta"
	appcat "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
)

const (
	kindEviction = "Eviction"
	PostgresName = "postgres-for-pgbouncer-test"
	pbVersion    = "1.10.0"
	reloadCMD    = "RELOAD"
	cityName     = "Dhaka"
	cityLocation = "random Co-ordinates"
)

func (i *Invocation) PgBouncer(secret *core.Secret) *api.PgBouncer {
	return &api.PgBouncer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rand.WithUniqSuffix(api.ResourceSingularPgBouncer),
			Namespace: i.namespace,
			Labels: map[string]string{
				"app": i.app,
			},
		},
		Spec: api.PgBouncerSpec{
			Version:  pbVersion,
			Replicas: types.Int32P(1),
			Databases: []api.Databases{
				{
					Alias:        api.ResourceSingularPostgres,
					DatabaseName: api.ResourceSingularPostgres,
					DatabaseRef: appcat.AppReference{
						Name:      PostgresName,
						Namespace: i.namespace,
					},
				},
			},

			ConnectionPool: &api.ConnectionPoolConfig{
				ReservePoolSize:      types.Int64P(ReservePoolSize),
				MaxClientConnections: types.Int64P(MaxClientConnections),
				AdminUsers: []string{
					"admin1",
				},
			},

			UserListSecretRef: &core.LocalObjectReference{
				Name: secret.Name,
			},
		},
	}
}

func (f *Framework) CreatePgBouncer(obj *api.PgBouncer) error {
	_, err := f.dbClient.KubedbV1alpha1().PgBouncers(obj.Namespace).Create(context.TODO(), obj, metav1.CreateOptions{})
	return err
}

func (f *Framework) GetPgBouncer(meta metav1.ObjectMeta) (*api.PgBouncer, error) {
	return f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
}

func (f *Framework) PatchPgBouncer(meta metav1.ObjectMeta, transform func(pgbouncer *api.PgBouncer) *api.PgBouncer) (*api.PgBouncer, error) {
	pgbouncer, err := f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	pgbouncer, _, err = util.PatchPgBouncer(context.TODO(), f.dbClient.KubedbV1alpha1(), pgbouncer, transform, metav1.PatchOptions{})
	return pgbouncer, err
}

func (f *Framework) DeletePgBouncer(meta metav1.ObjectMeta) error {
	return f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Delete(context.TODO(), meta.Name, meta_util.DeleteInForeground())
}

func (f *Framework) EventuallyPgBouncer(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			_, err := f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
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
			db, err := f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
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
			st, err := f.kubeClient.AppsV1beta1().StatefulSets(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
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
			pgbouncer, err := f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			return pgbouncer.Status.Phase == api.DatabasePhaseRunning
		},
		time.Minute*5,
		time.Second*5,
	)
}

func (f *Framework) EventuallyPodReadyForPgBouncer(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			st, err := f.kubeClient.AppsV1().StatefulSets(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			err = f.CheckStatefulSetPodStatus(st)
			return err == nil
		},
		time.Minute*15,
		time.Second*5,
	)
}

func (f *Framework) CheckStatefulSetPodStatus(statefulSet *apps.StatefulSet) error {
	err := WaitUntilPodRunningBySelector(
		f.kubeClient,
		statefulSet.Namespace,
		statefulSet.Spec.Selector,
		int(types.Int32(statefulSet.Spec.Replicas)),
	)
	if err != nil {
		return err
	}
	return nil
}

func WaitUntilPodRunningBySelector(kubeClient kubernetes.Interface, namespace string, selector *metav1.LabelSelector, count int) error {
	r, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return err
	}

	return wait.PollImmediate(kutil.RetryInterval, kutil.GCTimeout, func() (bool, error) {
		podList, err := kubeClient.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{
			LabelSelector: r.String(),
		})
		if err != nil {
			return true, nil
		}

		if len(podList.Items) != count {
			return true, nil
		}

		for _, pod := range podList.Items {
			runningAndReady, _ := core_util.PodRunningAndReady(pod)
			if !runningAndReady {
				return false, nil
			}
		}
		return true, nil
	})
}

func (f *Framework) CleanPgBouncer() {
	pgbouncerList, err := f.dbClient.KubedbV1alpha1().PgBouncers(f.namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return
	}
	for _, e := range pgbouncerList.Items {
		if _, _, err := util.PatchPgBouncer(context.TODO(), f.dbClient.KubedbV1alpha1(), &e, func(in *api.PgBouncer) *api.PgBouncer {
			in.ObjectMeta.Finalizers = nil
			//in.Spec.TerminationPolicy = api.TerminationPolicyWipeOut
			return in
		}, metav1.PatchOptions{}); err != nil {
			fmt.Printf("error Patching PgBouncer. error: %v", err)
		}
	}
	if err := f.dbClient.KubedbV1alpha1().PgBouncers(f.namespace).DeleteCollection(context.TODO(), meta_util.DeleteInForeground(), metav1.ListOptions{}); err != nil {
		fmt.Printf("error in deletion of PgBouncer. Error: %v", err)
	}
}

func (f *Framework) EventuallyWipedOut(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() error {
			labelMap := map[string]string{
				api.LabelDatabaseName: meta.Name,
				api.LabelDatabaseKind: api.ResourceKindPgBouncer,
			}
			labelSelector := labels.SelectorFromSet(labelMap)

			// check if pvcs is wiped out
			pvcList, err := f.kubeClient.CoreV1().PersistentVolumeClaims(meta.Namespace).List(
				context.TODO(),
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

			// check if secrets are wiped out
			secretList, err := f.kubeClient.CoreV1().Secrets(meta.Namespace).List(
				context.TODO(),
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
				context.TODO(),
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
