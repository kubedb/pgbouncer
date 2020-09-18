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

package framework

import (
	"context"
	"time"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kutil "kmodules.xyz/client-go"
)

const (
	operatorGetRetryInterval = time.Second * 5
)

func (f *Framework) SetupPostgresResources(kubeconfigPath string) {
	By("Setup postgres")
	postgres := f.Invoke().Postgres()
	_, err := f.GetPostgres(postgres.ObjectMeta)
	if kerr.IsNotFound(err) {
		err := f.CreatePostgres(postgres)
		Expect(err).ShouldNot(HaveOccurred())
	}
	By("Waiting for running Postgres")
	err = f.WaitUntilPostgresReady(postgres.Name)
	Expect(err).ShouldNot(HaveOccurred())
}

func (f *Framework) WaitUntilPostgresReady(name string) error {
	return wait.PollImmediate(operatorGetRetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		pg, err := f.dbClient.KubedbV1alpha1().Postgreses(f.Namespace()).Get(context.TODO(), name, metav1.GetOptions{})
		if err == nil {
			if pg.Status.Phase == api.DatabasePhaseRunning {
				return true, nil
			}
		}
		return false, nil
	})
}
