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

	. "github.com/onsi/gomega"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Framework) EventuallyAppBinding(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			_, err := f.appCatalogClient.AppBindings(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
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

func (f *Framework) CheckPostgresAppBindingSpec(meta metav1.ObjectMeta) error {
	postgres, err := f.GetPostgres(meta)
	Expect(err).NotTo(HaveOccurred())

	_, err = f.appCatalogClient.AppBindings(postgres.Namespace).Get(context.TODO(), postgres.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (f *Framework) CheckPgBouncerAppBindingSpec(meta metav1.ObjectMeta) error {
	pgbouncer, err := f.GetPgBouncer(meta)
	Expect(err).NotTo(HaveOccurred())

	_, err = f.appCatalogClient.AppBindings(pgbouncer.Namespace).Get(context.TODO(), pgbouncer.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	return nil
}
