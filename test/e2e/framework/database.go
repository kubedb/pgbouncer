package framework

import (
	"errors"
	"fmt"
	"github.com/appscode/go/log"
	shell "github.com/codeskyblue/go-sh"
	"github.com/go-xorm/xorm"
	_ "github.com/lib/pq"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kutil "kmodules.xyz/client-go"
	"kmodules.xyz/client-go/tools/portforward"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/pgbouncer/pkg/controller"
	"strings"
	"time"
)

const (
	testUser = "myuser"
	testPass = "mypass"
	testDB = "tmpdb"
)
func (f *Framework) ForwardPgBouncerPort(meta metav1.ObjectMeta) (*portforward.Tunnel, error) {
	pgbouncer, err := f.GetPgBouncer(meta)
	if err != nil {
		return nil, err
	}

	clientPodName := fmt.Sprintf("%v-0", pgbouncer.Name)
	tunnel := portforward.NewTunnel(
		f.kubeClient.CoreV1().RESTClient(),
		f.restConfig,
		pgbouncer.Namespace,
		clientPodName,
		controller.DefaultHostPort,
	)
	if err := tunnel.ForwardPort(); err != nil {
		return nil, err
	}
	return tunnel, nil
}

func (f *Framework) ForwardPort(meta metav1.ObjectMeta) (*portforward.Tunnel, error) {
	postgres, err := f.GetPostgres(meta)
	if err != nil {
		return nil, err
	}

	clientPodName := fmt.Sprintf("%v-0", postgres.Name)
	tunnel := portforward.NewTunnel(
		f.kubeClient.CoreV1().RESTClient(),
		f.restConfig,
		postgres.Namespace,
		clientPodName,
		controller.DefaultHostPort,
	)
	if err := tunnel.ForwardPort(); err != nil {
		return nil, err
	}
	return tunnel, nil
}

func (f *Framework) GetPostgresClient(tunnel *portforward.Tunnel, dbName string, userName string) (*xorm.Engine, error) {
	cnnstr := fmt.Sprintf("user=%s host=127.0.0.1 port=%v dbname=%s sslmode=disable", userName, tunnel.Local, dbName)
	return xorm.NewEngine("postgres", cnnstr)
}

func (f *Framework) EventuallyPingDatabase(meta metav1.ObjectMeta, dbName string, userName string) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			tunnel, err := f.ForwardPort(meta)
			if err != nil {
				return false
			}
			defer tunnel.Close()

			db, err := f.GetPostgresClient(tunnel, dbName, userName)
			if err != nil {
				return false
			}
			defer db.Close()

			if err := f.CheckPostgres(db); err != nil {
				return false
			}
			return true
		},
		time.Minute*10,
		time.Second*5,
	)
}

func (f *Framework) EventuallyPingPgBouncerServer(meta metav1.ObjectMeta) error {
	return wait.PollImmediate(operatorGetRetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		tunnel, err := f.ForwardPgBouncerPort(meta)
		if err != nil {
			log.Infoln("Port forward err: ", err)
			return false, nil
		}
		defer tunnel.Close()
		pingResult := f.PingPgBouncerServer(tunnel.Local)
		return pingResult, nil
	})
}

func (f *Framework) PingPgBouncerServer(port int) bool {
	sh := shell.NewSession()
	pgbouncer := api.ResourceSingularPgBouncer
	cmd := sh.Command("docker", "run",
		"-e",fmt.Sprintf("%s=%s","PGPASSWORD","kubedb"),
		"--network=host",
		"postgres:11.1-alpine", "psql",
		"--host=localhost", fmt.Sprintf("--port=%d", port),
		fmt.Sprintf("--username=%s", "kubedb"), pgbouncer, "--command=RELOAD")
	out, err := cmd.Output()
	if err != nil {
		log.Infoln("CMD out err = ", err)
		return false
	}
	outText := strings.TrimSpace(string(out))
	if outText != CmdReload {
		return false
	}
	return true
}

func (f *Framework) CheckPgBouncer(db *xorm.Engine) error {
	err := db.Ping()
	if err != nil {
		return err
	}
	return nil
}

func (f *Framework) CheckPostgres(db *xorm.Engine) error {
	err := db.Ping()
	if err != nil {
		return err
	}
	return nil
}

type PgStatArchiver struct {
	ArchivedCount int
}

func (f *Framework) PoolViaPgBouncer(meta metav1.ObjectMeta) error {
	err := f.CheckSecret()
	if err != nil {
		return err
	}
	username, password, err := f.GetPostgresCredentials()
	if err != nil {
		return err
	}
	tunnel, err := f.ForwardPgBouncerPort(meta)
	if err != nil {
		return err
	}
	defer tunnel.Close()

	err = f.CreateTableViaPgBouncer(username, password, api.ResourceSingularPostgres, tunnel.Local)
	if err != nil {
		return err
	}
	err = f.CheckTableViaPgBouncer(username, password, api.ResourceSingularPostgres, tunnel.Local)
	if err != nil {
		return err
	}
	err = f.DropTableViaPgBouncer(username, password, api.ResourceSingularPostgres, tunnel.Local)
	if err != nil {
		return err
	}
	return nil
}

func (f *Framework) CreateTableViaPgBouncer(username, password, dbName string, port int) error {
	sqlCommand := "CREATE TABLE cities (name varchar(80), location varchar(80));"

	outText, err := f.ApplyCMD(username, password, sqlCommand, dbName, port)
	if err != nil {
		return err
	}
	if outText != "CREATE TABLE" {
		return errors.New("can't create table")
	}
	sqlCommand = fmt.Sprintf("INSERT INTO cities (name, location) VALUES ('%s','%s');", cityName, cityLocation)
	_, err = f.ApplyCMD(username, password, sqlCommand, dbName, port)
	if err != nil {
		return err
	}
	return nil
}

func (f *Framework) CheckTableViaPgBouncer(username, password, dbName string, port int) error {
	sqlCommand := "SELECT * FROM cities ORDER BY name;"
	outText, err := f.ApplyCMD(username, password, sqlCommand, dbName, port)
	if err != nil {
		return err
	}
	if !strings.Contains(outText, cityName) || !strings.Contains(outText, cityLocation) {
		return errors.New("can't find data")
	}
	return nil
}

func (f *Framework) DropTableViaPgBouncer(username, password, dbName string, port int) error {
	sqlCommand := "DROP TABLE cities;"
	outText, err := f.ApplyCMD(username, password, sqlCommand, dbName, port)
	if err != nil {
		return err
	}
	if outText != "DROP TABLE" {
		return errors.New("can't drop table")
	}
	return nil
}

func (f *Framework) CreateUserAndDatabaseViaPgBouncer(meta metav1.ObjectMeta) error {
	err := f.CheckSecret()
	if err != nil {
		return err
	}
	username, password, err := f.GetPostgresCredentials()
	if err != nil {
		return err
	}
	tunnel, err := f.ForwardPgBouncerPort(meta)
	if err != nil {
		return err
	}
	defer tunnel.Close()
	//Create Database tmpdb in postgres
	err = f.CreateDatabaseViaPgBouncer(username, password,api.ResourceSingularPostgres ,tunnel.Local)
	if err != nil {
		return err
	}
	//create user myuser in postgres
	err = f.CreateUserViaPgBouncer(username, password, api.ResourceSingularPostgres, tunnel.Local)
	if err != nil {
		return err
	}

	//Add database info to pgbouncer
	_, err = f.PatchPgBouncer(meta, func(in *api.PgBouncer) *api.PgBouncer {
		tmpDB := api.Databases{
			Alias:testDB,
			DbName:testDB,
			AppBindingName:PostgresName,
		}
		in.Spec.Databases = append(in.Spec.Databases, tmpDB)
		return in
	})
	if err != nil {
		return err
	}
	err = f.waitUntilPatchedConfigMapReady(meta)
	if err != nil {
		return err
	}

	err = f.CreateTableViaPgBouncer(testUser, testPass, testDB, tunnel.Local)
	if err != nil {
		return err
	}

	err = f.CheckTableViaPgBouncer(testUser, testPass, testDB, tunnel.Local)
	if err != nil {
		return err
	}

	err = f.DropTableViaPgBouncer(testUser, testPass, testDB, tunnel.Local)
	if err != nil {
		return err
	}

	return nil
}

func (f *Framework) CreateDatabaseViaPgBouncer(username, password, dbName string, port int) error {
	sqlCommand := fmt.Sprintf("CREATE DATABASE %s;",testDB)
	outText, err := f.ApplyCMD(username, password, sqlCommand, dbName, port)
	if err != nil {
		return err
	}
	if outText != "CREATE DATABASE" {
		return errors.New("can't create database")
	}
	return nil
}
func (f *Framework) CreateUserViaPgBouncer(username, password, dbName string, port int) error {
	sqlCommand := fmt.Sprintf("create user %s with encrypted password '%s';",testUser,testPass)
	outText, err := f.ApplyCMD(username, password, sqlCommand, api.ResourceSingularPostgres, port)
	if err != nil {
		return err
	}
	if outText != "CREATE ROLE" {
		return errors.New("can't create user")
	}
	return nil
}

func (f *Framework) ApplyCMD(username, password, sqlCommand, dbName string, port int) (string, error) {
	sh := shell.NewSession()
	cmd := sh.Command("docker", "run",
		"-e",fmt.Sprintf("%s=%s","PGPASSWORD",password),
		"--network=host",
		"postgres:11.1-alpine", "psql",
		"--host=localhost", fmt.Sprintf("--port=%d", port),
		fmt.Sprintf("--username=%s", username), dbName, fmt.Sprintf("--command=%s", sqlCommand))
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	outText := strings.TrimSpace(string(out))
	return outText, nil
}

func (f *Framework) waitUntilPatchedConfigMapReady( meta metav1.ObjectMeta) error {
	return wait.PollImmediate(kutil.RetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		service, err := f.kubeClient.CoreV1().Services(meta.Namespace).Get(meta.Name,metav1.GetOptions{})
		print(".")
		if err != nil {
			return false, err
		}
		antn := service.GetObjectMeta().GetAnnotations()
		if  antn["podConfigMap"] =="patched"{
			println(". Done!")
			return true, nil
		}
		return false, nil
	})
}
