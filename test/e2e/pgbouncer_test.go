package e2e_test

import (
	"time"

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
	PostgresName            = "postgres-for-pgbouncer-test"
)

var _ = Describe("PgBouncer", func() {
	var (
		err              error
		f                *framework.Invocation
		pgbouncer        *api.PgBouncer
		postgres         *api.Postgres
		garbagePgBouncer *api.PgBouncerList
		secret           *core.Secret
		skipMessage      string

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
		skipMessage = ""
	})

	var createAndWaitForRunning = func() {
		By("Creating Postgres: " + postgres.Name)
		err = f.CreatePostgres(postgres)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for Running Postgres")
		f.EventuallyPostgresRunning(postgres.ObjectMeta).Should(BeTrue())

		By("Creating PgBouncer: " + pgbouncer.Name)
		err = f.CreatePgBouncer(pgbouncer)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for Running pgbouncer")
		f.EventuallyPgBouncerRunning(pgbouncer.ObjectMeta).Should(BeTrue())

		By("Waiting for database to be ready")
		f.EventuallyPingDatabase(pgbouncer.ObjectMeta, dbName, dbUser).Should(BeTrue())

		By("Wait for AppBinding to create")
		f.EventuallyAppBinding(postgres.ObjectMeta).Should(BeTrue())
		f.EventuallyAppBinding(pgbouncer.ObjectMeta).Should(BeTrue())

		By("Check valid AppBinding Specs")
		err := f.CheckPostgresAppBindingSpec(postgres.ObjectMeta)
		Expect(err).NotTo(HaveOccurred())
		err = f.CheckPgBouncerAppBindingSpec(pgbouncer.ObjectMeta)
		Expect(err).NotTo(HaveOccurred())
	}

	var testGeneralBehaviour = func() {
		if skipMessage != "" {
			Skip(skipMessage)
		}
		// Create PgBouncer
		createAndWaitForRunning()

		By("Creating Schema")
		f.EventuallyCreateSchema(pgbouncer.ObjectMeta, dbName, dbUser).Should(BeTrue())

		By("Creating Table")
		f.EventuallyCreateTable(pgbouncer.ObjectMeta, dbName, dbUser, 3).Should(BeTrue())

		By("Checking Table")
		f.EventuallyCountTable(pgbouncer.ObjectMeta, dbName, dbUser).Should(Equal(3))

		By("Delete pgbouncer")
		err = f.DeletePgBouncer(pgbouncer.ObjectMeta)
		Expect(err).NotTo(HaveOccurred())

		// Create PgBouncer object again to resume it
		By("Create PgBouncer: " + pgbouncer.Name)
		err = f.CreatePgBouncer(pgbouncer)
		Expect(err).NotTo(HaveOccurred())

		By("Wait for Running pgbouncer")
		f.EventuallyPgBouncerRunning(pgbouncer.ObjectMeta).Should(BeTrue())

		By("Checking Table")
		f.EventuallyCountTable(pgbouncer.ObjectMeta, dbName, dbUser).Should(Equal(3))
	}

	var deleteTestResource = func() {
		if pgbouncer == nil {
			Skip("Skipping")
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

			FContext("Test Operator", func() {
				PrintSomething := func() {

					pg, err := f.GetPostgres(postgres.ObjectMeta)
					if err != nil {

						println("err =", err)
					}
					println("+++++++++", pg.Namespace+" "+pg.Name, pg.Status.Phase)
					time.Sleep(time.Second * 30)
					println("===========", pg.Namespace+" "+pg.Name, pg.Status.Phase)
					println("::::::::Done:::::::")

				}

				It("Should ping postgres", PrintSomething)
			})
			Context("With PVC", func() {

				It("should run successfully", testGeneralBehaviour)
			})

			Context("PDB", func() {

				It("should run evictions successfully", func() {
					// Create PgBouncer
					pgbouncer.Spec.Replicas = types.Int32P(3)
					createAndWaitForRunning()
					//Evict a PgBouncer pod
					By("Try to evict Pods")
					err := f.EvictPodsFromStatefulSet(pgbouncer.ObjectMeta)
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})
		//Context("EnvVars", func() {
		//
		//	Context("With all supported EnvVars", func() {
		//
		//		It("should create DB with provided EnvVars", func() {
		//			if skipMessage != "" {
		//				Skip(skipMessage)
		//			}
		//
		//			const (
		//				dataDir = "/var/pv/pgdata"
		//				walDir  = "/var/pv/wal"
		//			)
		//			dbName = f.App()
		//			pgbouncer.Spec.PodTemplate.Spec.Env = []core.EnvVar{
		//				{
		//					Name:  PGDATA,
		//					Value: dataDir,
		//				},
		//				{
		//					Name:  POSTGRES_DB,
		//					Value: dbName,
		//				},
		//				{
		//					Name:  POSTGRES_INITDB_ARGS,
		//					Value: "--data-checksums",
		//				},
		//			}
		//
		//			walEnv := []core.EnvVar{
		//				{
		//					Name:  POSTGRES_INITDB_WALDIR,
		//					Value: walDir,
		//				},
		//			}
		//
		//			if strings.HasPrefix(framework.DBCatalogName, "9") {
		//				walEnv = []core.EnvVar{
		//					{
		//						Name:  POSTGRES_INITDB_XLOGDIR,
		//						Value: walDir,
		//					},
		//				}
		//			}
		//			pgbouncer.Spec.PodTemplate.Spec.Env = core_util.UpsertEnvVars(pgbouncer.Spec.PodTemplate.Spec.Env, walEnv...)
		//
		//			// Run PgBouncer with provided Environment Variables
		//			testGeneralBehaviour()
		//		})
		//	})
		//
		//	Context("Root Password as EnvVar", func() {
		//
		//		It("should reject to create PgBouncer CRD", func() {
		//			if skipMessage != "" {
		//				Skip(skipMessage)
		//			}
		//
		//			dbName = f.App()
		//			pgbouncer.Spec.PodTemplate.Spec.Env = []core.EnvVar{
		//				{
		//					Name:  POSTGRES_PASSWORD,
		//					Value: "not@secret",
		//				},
		//			}
		//
		//			By("Creating Posgres: " + pgbouncer.Name)
		//			err = f.CreatePgBouncer(pgbouncer)
		//			Expect(err).To(HaveOccurred())
		//		})
		//	})
		//
		//	Context("Update EnvVar", func() {
		//
		//		It("should not reject to update EnvVar", func() {
		//			if skipMessage != "" {
		//				Skip(skipMessage)
		//			}
		//
		//			dbName = f.App()
		//			pgbouncer.Spec.PodTemplate.Spec.Env = []core.EnvVar{
		//				{
		//					Name:  POSTGRES_DB,
		//					Value: dbName,
		//				},
		//			}
		//
		//			// Run PgBouncer with provided Environment Variables
		//			testGeneralBehaviour()
		//
		//			By("Patching EnvVar")
		//			_, _, err = util.PatchPgBouncer(f.ExtClient().KubedbV1alpha1(), pgbouncer, func(in *api.PgBouncer) *api.PgBouncer {
		//				in.Spec.PodTemplate.Spec.Env = []core.EnvVar{
		//					{
		//						Name:  POSTGRES_DB,
		//						Value: "patched-db",
		//					},
		//				}
		//				return in
		//			})
		//			Expect(err).NotTo(HaveOccurred())
		//		})
		//	})
		//})
	})
})
