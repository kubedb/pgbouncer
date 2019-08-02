package framework

import (
	"time"

	. "github.com/onsi/gomega"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *Framework) EventuallyAppBinding(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			_, err := f.appCatalogClient.AppBindings(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
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

	_, err = f.appCatalogClient.AppBindings(postgres.Namespace).Get(postgres.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (f *Framework) CheckPgBouncerAppBindingSpec(meta metav1.ObjectMeta) error {
	pgbouncer, err := f.GetPgBouncer(meta)
	Expect(err).NotTo(HaveOccurred())

	_, err = f.appCatalogClient.AppBindings(pgbouncer.Namespace).Get(pgbouncer.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	return nil
}
