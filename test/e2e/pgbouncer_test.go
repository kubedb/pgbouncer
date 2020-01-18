/*
Copyright The KubeDB Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e_test

import (
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/pgbouncer/test/e2e/framework"

	"github.com/appscode/go/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
)

var _ = Describe("PgBouncer", func() {
	var (
		err            error
		f              *framework.Invocation
		pgbouncer      *api.PgBouncer
		userlistSecret *core.Secret
		clientCASecret *core.Secret
	)

	BeforeEach(func() {
		f = root.Invoke()
		userlistSecret = f.GetUserListSecret()
		pgbouncer = f.PgBouncer(userlistSecret)
		clientCASecret = nil
	})

	var createAndRunPgBouncer = func() {
		By("Create userList secret")
		err := f.CreateSecret(userlistSecret)
		Expect(err).NotTo(HaveOccurred())
		By("Create PgBouncer")
		err = f.CreatePgBouncer(pgbouncer)
		Expect(err).NotTo(HaveOccurred())
		By("Wait for Running PgBouncer")
		f.EventuallyPgBouncerRunning(pgbouncer.ObjectMeta).Should(BeTrue())
		By("Wait for Pods to be available")
		f.EventuallyPgBouncerPodCount(pgbouncer.ObjectMeta)
		By("Wait for primary container to be ready")
		err = f.WaitUntilPrimaryContainerReady(pgbouncer.ObjectMeta)
		Expect(err).NotTo(HaveOccurred())
	}

	var deleteTestResource = func() {
		if pgbouncer == nil {
			Skip("Skipping")
		}

		By("Delete " + pgbouncer.Name)
		err = f.DeletePgBouncer(pgbouncer.ObjectMeta)
		if err != nil {
			Expect(err).NotTo(HaveOccurred())
		}

		By("Check if PgBouncer " + pgbouncer.Name + " exists.")
		_, err := f.GetPgBouncer(pgbouncer.ObjectMeta)
		if err != nil {
			if kerr.IsNotFound(err) {
				// PgBouncer was not created. Hence, rest of cleanup is not necessary.
				return
			}
			Expect(err).NotTo(HaveOccurred())
		}

		By("Check if userlist secret exists.")
		err = f.CheckUserListSecret(userlistSecret.ObjectMeta)
		if err != nil {
			if !kerr.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		} else {
			By("Delete userlist secret")
			err = f.DeleteUserListSecret(userlistSecret.ObjectMeta)
			Expect(err).NotTo(HaveOccurred())
		}

		By("Wait for PgBouncer resources to be wipedOut")
		f.EventuallyWipedOut(pgbouncer.ObjectMeta).Should(Succeed())
	}

	AfterEach(func() {
		if clientCASecret != nil {
			err := f.DeleteSecret(clientCASecret.ObjectMeta)
			Expect(err).NotTo(HaveOccurred())
		}
		// Delete test resource
		deleteTestResource()
	})

	Describe("Test", func() {
		Context("General", func() {
			It("Should ping PgBouncer", func() {
				createAndRunPgBouncer()
				By("Ping PgBouncer")
				err = f.EventuallyPingPgBouncerServer(pgbouncer.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("PDB", func() {
			It("should run evictions successfully", func() {
				// Create PgBouncer
				pgbouncer.Spec.Replicas = types.Int32P(3)
				createAndRunPgBouncer()
				//Evict a PgBouncer pod
				By("Evict pods")
				err := f.EvictPgBouncerPods(pgbouncer.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("Exporter", func() {
			It("should export selected metrics", func() {
				By("Add monitoring configurations to pgbouncer")
				f.AddMonitor(pgbouncer)
				// Create PgBouncer
				createAndRunPgBouncer()
				By("Verify exporter")
				err = f.VerifyExporter(pgbouncer.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("Pooling", func() {
			It("Should write to, and read from database", func() {
				By("Create and Run PgBouncer")
				createAndRunPgBouncer()
				By("Check Pooling via PgBouncer")
				err := f.PoolViaPgBouncer(pgbouncer.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())
			})
			It("Should add new user and database", func() {
				pgbouncer = f.PgBouncer(userlistSecret)
				By("Create and run PgBouncer")
				createAndRunPgBouncer()
				By("Check connection-pooling via PgBouncer")
				err := f.CreateUserAndDatabaseViaPgBouncer(pgbouncer.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())
			})

		})
	})
})
