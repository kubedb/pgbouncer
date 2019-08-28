package e2e_test

import (
	"github.com/appscode/go/log"
	"github.com/appscode/go/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	core "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/pgbouncer/test/e2e/framework"
)

const (
	S3_BUCKET_NAME          = "S3_BUCKET_NAME"
	GCS_BUCKET_NAME         = "GCS_BUCKET_NAME"
	AZURE_CONTAINER_NAME    = "AZURE_CONTAINER_NAME"
	SWIFT_CONTAINER_NAME    = "SWIFT_CONTAINER_NAME"
	POSTGRES_DB             = "POSTGRES_DB"
	POSTGRES_PASSWORD       = "POSTGRES_PASSWORD"
	PGDATA                  = "PGDATA"
	POSTGRES_USER           = "POSTGRES_USER"
	POSTGRES_INITDB_ARGS    = "POSTGRES_INITDB_ARGS"
	POSTGRES_INITDB_WALDIR  = "POSTGRES_INITDB_WALDIR"
	POSTGRES_INITDB_XLOGDIR = "POSTGRES_INITDB_XLOGDIR"
	POSTGRES_NAME            = "postgres-for-pgbouncer-test"
)

var _ = Describe("PgBouncer", func() {
	var (
		err              error
		f                *framework.Invocation
		pgbouncer        *api.PgBouncer
		postgres         *api.Postgres
		garbagePgBouncer *api.PgBouncerList
		secret           *core.Secret

		dbName string
		dbUser string
	)
	//var kubeConfigFile = filepath.Join(homedir.HomeDir(), ".kube/config")
	//var uninstallKubeDB = func() {
	//	err := sh.Command("bash", "scripts/kubedb.sh", "--uninstall", "--purge", "--kubeconfig="+kubeConfigFile).Run()
	//	Expect(err).NotTo(HaveOccurred())
	//}
	//
	//var installKubeDB = func() {
	//	err := sh.Command("bash", "scripts/kubedb.sh", "--kubeconfig="+kubeConfigFile).Run()
	//	Expect(err).NotTo(HaveOccurred())
	//	time.Sleep(time.Minute * 5)
	//	uninstallKubeDB()
	//}

	//var uninstallKubeDB = func() {
	//	err := sh.Command("bash", "scripts/kubedb.sh", "--uninstall", "--purge", "--kubeconfig="+kubeConfigFile).Run()
	//	Expect(err).NotTo(HaveOccurred())
	//}

	BeforeEach(func() {
		f = root.Invoke()
		pgbouncer = f.PgBouncer()
		postgres = f.Postgres()
		garbagePgBouncer = new(api.PgBouncerList)
		secret = nil
		dbName = "postgres"
		dbUser = "postgres"
	})

	var createAndRunPgBouncer = func() {
		By("Addind postgres to userlist from: " + postgres.Name)
		err := f.CreateUserListSecret()
		Expect(err).NotTo(HaveOccurred())
		By("Creating PgBouncer: " + pgbouncer.Name)
		err = f.CreatePgBouncer(pgbouncer)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for Running PgBouncer")
		f.EventuallyPgBouncerRunning(pgbouncer.ObjectMeta).Should(BeTrue())
	}

	var checkPostgres = func() {
		By("Waiting for database to be ready")
		f.EventuallyPingDatabase(postgres.ObjectMeta, dbName, dbUser).Should(BeTrue())

		By("Wait for AppBindings to create")
		f.EventuallyAppBinding(postgres.ObjectMeta).Should(BeTrue())
		//f.EventuallyAppBinding(pgbouncer.ObjectMeta).Should(BeTrue())

		By("Check valid AppBinding Spec")
		err := f.CheckPostgresAppBindingSpec(postgres.ObjectMeta)
		Expect(err).NotTo(HaveOccurred())
	}

	var deleteTestResource = func() {
		if pgbouncer == nil {
			Skip("Skipping")
		}
		By("Check if userlist secret exists.")
		err  = f.CheckUserListSecret()
		if err != nil {
			if !kerr.IsNotFound(err){
				Expect(err).NotTo(HaveOccurred())
			}
		}else {
			By("Delete  userlist secret")
			err  = f.DeleteUserListSecret()
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

		By("Delete pgbouncer: " + pgbouncer.Name)
		err = f.DeletePgBouncer(pgbouncer.ObjectMeta)
		if err != nil {
			if kerr.IsNotFound(err) {
				// PgBouncer was not created. Hence, rest of cleanup is not necessary.
				log.Infof("Skipping rest of cleanup. Reason: PgBouncer %s is not found.", pgbouncer.Name)
				return
			}
			Expect(err).NotTo(HaveOccurred())
		}

		By("Wait for pgbouncer resources to be wipedOut")
		f.EventuallyWipedOut(pgbouncer.ObjectMeta).Should(Succeed())
	}

	AfterEach(func() {
		// Delete test resource
		deleteTestResource()

		for _, pg := range garbagePgBouncer.Items {
			*pgbouncer = pg
			// Delete test resource
			deleteTestResource()
		}

		if secret != nil {
			err := f.DeleteSecret(secret.ObjectMeta)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	Describe("Test", func() {

		Context("General", func() {
			It("Should have a running postgres", checkPostgres)
			It("Should ping pgbouncer server", func() {
				createAndRunPgBouncer()
				By("Ping PgBouncer")
				err = f.EventuallyPingPgBouncerServer(pgbouncer.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("Connection Pool", func() {
			It("Should write to, and read from database", func() {
				By("Create and Run PgBouncer")
				createAndRunPgBouncer()
				By("Check for existing postgres")
				checkPostgres()
				By("Check Pooling via PgBouncer")
				err := f.PoolViaPgBouncer(pgbouncer.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())
			})
			It("Should add new user and database", func() {
				By("Create and Run PgBouncer")
				createAndRunPgBouncer()
				By("Check for existing postgres")
				checkPostgres()
				By("Check Pooling via PgBouncer")
				err := f.CreateUserAndDatabaseViaPgBouncer(pgbouncer.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())

			})

		})

		Context("PDB", func() {

			It("should run evictions successfully", func() {
				// Create PgBouncer
				pgbouncer.Spec.Replicas = types.Int32P(3)
				createAndRunPgBouncer()
				//Evict a PgBouncer pod
				By("Evict Pods")
				err := f.EvictPgBouncerPods(pgbouncer.ObjectMeta)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})
})
