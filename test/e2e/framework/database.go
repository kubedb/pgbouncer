package framework

import (
	"errors"
	"fmt"
	"strings"
	"time"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/pgbouncer/pkg/controller"

	"github.com/appscode/go/log"
	shell "github.com/codeskyblue/go-sh"
	"github.com/go-xorm/xorm"
	_ "github.com/lib/pq"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kutil "kmodules.xyz/client-go"
	"kmodules.xyz/client-go/tools/exec"
	"kmodules.xyz/client-go/tools/portforward"
	appcat "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
)

const (
	testUser = "myuser"
	testPass = "mypass"
	testDB   = "tmpdb"
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
		pingResult := f.PingPgBouncerServer(meta, tunnel.Local)
		return pingResult, nil
	})
}

func (f *Framework) PingPgBouncerServer(meta metav1.ObjectMeta, port int) bool {
	pod, err := f.kubeClient.CoreV1().Pods(meta.Namespace).Get(meta.Name+"-0", metav1.GetOptions{})
	if err != nil {
		log.Infoln(err)
		return false
	}
	options := []func(options *exec.Options){
		exec.Command([]string{"cat", "/var/run/pgbouncer/secret/pb-password)"}...),
	}
	outT, err := exec.ExecIntoPod(f.restConfig, pod, options...)
	println(outT)

	sh := shell.NewSession()
	cmd := sh.Command("kubectl", "exec", "-i",
		"-n", meta.Namespace, fmt.Sprintf("%s-0", meta.Name),
		"-c", api.ResourceSingularPgBouncer, "--",
		"cat", "/var/run/pgbouncer/secret/pb-password")
	out, err := cmd.Output()
	if err != nil {
		log.Infoln(err)
		return false
	}
	password := strings.TrimSpace(string(out))
	pgbouncer := api.ResourceSingularPgBouncer
	cmd = sh.Command("docker", "run",
		"-e", fmt.Sprintf("%s=%s", "PGPASSWORD", password),
		"--network=host",
		"postgres:11.1-alpine", "psql",
		"--host=localhost", fmt.Sprintf("--port=%d", port),
		fmt.Sprintf("--username=%s", "kubedb"), pgbouncer, "--command=RELOAD")
	out, err = cmd.Output()
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
	err = f.CreateDatabaseViaPgBouncer(username, password, api.ResourceSingularPostgres, tunnel.Local)
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
			Alias:        testDB,
			DatabaseName: testDB,
			DatabaseRef: appcat.AppReference{
				Name: PostgresName,
			},
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
	sqlCommand := fmt.Sprintf("CREATE DATABASE %s;", testDB)
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
	sqlCommand := fmt.Sprintf("create user %s with encrypted password '%s';", testUser, testPass)
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
		"-e", fmt.Sprintf("%s=%s", "PGPASSWORD", password),
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

func (f *Framework) waitUntilPatchedConfigMapReady(meta metav1.ObjectMeta) error {
	sh := shell.NewSession()

	return wait.PollImmediate(time.Second, kutil.ReadinessTimeout, func() (bool, error) {
		cmd := sh.Command("kubectl", "exec", "-i",
			"-n", meta.Namespace, fmt.Sprintf("%s-0", meta.Name),
			"-c", api.ResourceSingularPgBouncer, "--",
			"cat", "/var/run/pgbouncer/pb_status")
		out, err := cmd.Output()
		if err != nil {
			return false, err
		}
		outText := strings.TrimSpace(string(out))

		if outText == "RELOADED" {
			println(". Done!")
			return true, nil
		}
		return false, nil
	})
}

/*
	pod, err := f.kubeClient.CoreV1().Pods(meta.Namespace).Get(meta.Name+"-0", metav1.GetOptions{})
	if err != nil {
		log.Infoln(err)
		return false
	}
	pgbouncer := api.ResourceSingularPgBouncer
	options := []func(options *exec.Options){
		exec.Command([]string{"env","PGPASSWORD=$(cat /var/run/pgbouncer/secret/pb-password)","psql",
		"--host=localhost", fmt.Sprintf("--port=%d", port),
		fmt.Sprintf("--username=%s", "kubedb"), pgbouncer, "--command=RELOAD"}...),
	}
	outText, err := exec.ExecIntoPod(f.restConfig, pod, options...)
	println(outText)
	if outText != CmdReload {
		return false
	}
	return true

*/
