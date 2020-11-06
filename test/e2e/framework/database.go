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
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha2"
	"kubedb.dev/pgbouncer/pkg/controller"

	"github.com/go-xorm/xorm"
	_ "github.com/lib/pq"
	. "github.com/onsi/gomega"
	"gomodules.xyz/pointer"
	v1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
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

func (f *Framework) ForwardPort(meta metav1.ObjectMeta, port *int) (*portforward.Tunnel, error) {
	var defaultPort = api.PgBouncerDatabasePort
	if port != nil {
		defaultPort = *port
	}

	clientPodName := fmt.Sprintf("%v-0", meta.Name)
	tunnel := portforward.NewTunnel(
		f.kubeClient.CoreV1().RESTClient(),
		f.restConfig,
		meta.Namespace,
		clientPodName,
		defaultPort,
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
			tunnel, err := f.ForwardPort(meta, nil)
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
		return f.PingPgBouncerServer(meta)
	})
}
func (f *Framework) EventuallyPingPgBouncerServerOverTLS(meta metav1.ObjectMeta) error {
	return wait.PollImmediate(operatorGetRetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		return f.PingPgBouncerServerOverTLS(meta)
	})
}

func (f *Framework) PingPgBouncerServer(meta metav1.ObjectMeta) (bool, error) {
	pod, password, port32, err := f.getPodPassPort(meta)
	if err != nil {
		return false, nil
	}
	port := int(*port32)

	pgbouncer := api.ResourceSingularPgBouncer
	outText, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command("env", fmt.Sprintf("PGPASSWORD=%s", password),
		"psql", "--host=localhost", fmt.Sprintf("--port=%d", port), "--user=kubedb", pgbouncer, "--command=RELOAD"))
	if err != nil {
		return false, nil
	}

	return strings.TrimSpace(outText) == reloadCMD, nil
}

func (f *Framework) PingPgBouncerServerOverTLS(meta metav1.ObjectMeta) (bool, error) {
	pod, password, port32, err := f.getPodPassPort(meta)
	if err != nil {
		return false, nil
	}

	port := int(*port32)
	pgbouncer := api.ResourceSingularPgBouncer

	outText, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command("psql",
		fmt.Sprintf("host=localhost port=%d user=kubedb password=%s dbname=%s sslcert=%s sslkey=%s sslrootcert=%s sslmode=verify-full",
			port, password, pgbouncer,
			filepath.Join(controller.ServingCertMountPath, string(api.PgBouncerClientCert), "tls.crt"),
			filepath.Join(controller.ServingCertMountPath, string(api.PgBouncerClientCert), "tls.key"),
			filepath.Join(controller.ServingCertMountPath, string(api.PgBouncerClientCert), "ca.crt")),
		"--command=RELOAD"))
	if err != nil {
		return false, nil
	}

	return strings.TrimSpace(outText) == reloadCMD, nil
}

func (f *Framework) CheckPostgres(db *xorm.Engine) error {
	return db.Ping()
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

	err = f.CreateTableViaPgBouncer(meta, username, password, api.ResourceSingularPostgres)
	if err != nil {
		return err
	}
	err = f.CheckTableViaPgBouncer(meta, username, password, api.ResourceSingularPostgres)
	if err != nil {
		return err
	}
	err = f.DropTableViaPgBouncer(meta, username, password, api.ResourceSingularPostgres)
	if err != nil {
		return err
	}

	return nil
}

func (f *Framework) CreateTableViaPgBouncer(meta metav1.ObjectMeta, username, password, dbName string) error {
	sqlCommand := "CREATE TABLE cities (name varchar(80), location varchar(80));"
	outText, err := f.ApplyCMD(meta, username, password, sqlCommand, dbName)
	if err != nil {
		return err
	}
	if outText != "CREATE TABLE" {
		return errors.New("can't create table")
	}
	sqlCommand = fmt.Sprintf("INSERT INTO cities (name, location) VALUES ('%s','%s');", cityName, cityLocation)
	_, err = f.ApplyCMD(meta, username, password, sqlCommand, dbName)
	if err != nil {
		return err
	}

	return nil
}

func (f *Framework) CheckTableViaPgBouncer(meta metav1.ObjectMeta, username, password, dbName string) error {
	sqlCommand := "SELECT * FROM cities ORDER BY name;"
	outText, err := f.ApplyCMD(meta, username, password, sqlCommand, dbName)
	if err != nil {
		return err
	}
	if !strings.Contains(outText, cityName) || !strings.Contains(outText, cityLocation) {
		return errors.New("can't find data")
	}

	return nil
}

func (f *Framework) DropTableViaPgBouncer(meta metav1.ObjectMeta, username, password, dbName string) error {
	sqlCommand := "DROP TABLE cities;"
	outText, err := f.ApplyCMD(meta, username, password, sqlCommand, dbName)
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

	//Create Database tmpdb in postgres
	err = f.CreateDatabaseViaPgBouncer(meta, username, password, api.ResourceSingularPostgres)
	if err != nil {
		return err
	}
	//create user myuser in postgres
	err = f.CreateUserViaPgBouncer(meta, username, password, api.ResourceSingularPostgres)
	if err != nil {
		return err
	}
	//Add database info to pgbouncer
	_, err = f.PatchPgBouncer(meta, func(in *api.PgBouncer) *api.PgBouncer {
		tmpDB := api.Databases{
			Alias:        testDB,
			DatabaseName: testDB,
			DatabaseRef: appcat.AppReference{
				Name:      PostgresName,
				Namespace: meta.Namespace,
			},
		}
		in.Spec.Databases = append(in.Spec.Databases, tmpDB)
		return in
	})
	if err != nil {
		return err
	}
	if err = f.waitUntilPatchedConfigMapReady(meta); err != nil {
		return err
	}

	if err = f.CreateTableViaPgBouncer(meta, testUser, testPass, testDB); err != nil {
		return err
	}

	err = f.CheckTableViaPgBouncer(meta, testUser, testPass, testDB)
	if err != nil {
		return err
	}

	err = f.DropTableViaPgBouncer(meta, testUser, testPass, testDB)
	if err != nil {
		return err
	}

	return nil
}

func (f *Framework) CreateDatabaseViaPgBouncer(meta metav1.ObjectMeta, username, password, dbName string) error {
	sqlCommand := fmt.Sprintf("CREATE DATABASE %s;", testDB)
	outText, err := f.ApplyCMD(meta, username, password, sqlCommand, dbName)
	if err != nil {
		return err
	}
	if outText != "CREATE DATABASE" {
		return errors.New("can't create database")
	}

	return nil
}

func (f *Framework) CreateUserViaPgBouncer(meta metav1.ObjectMeta, username, password, dbName string) error {
	sqlCommand := fmt.Sprintf("create user %s with encrypted password '%s';", testUser, testPass)
	outText, err := f.ApplyCMD(meta, username, password, sqlCommand, api.ResourceSingularPostgres)
	if err != nil {
		return err
	}
	if outText != "CREATE ROLE" {
		return errors.New("can't create user")
	}

	return nil
}

func (f *Framework) ApplyCMD(meta metav1.ObjectMeta, username, password, sqlCommand, dbName string) (string, error) {
	pod, _, port32, err := f.getPodPassPort(meta)
	if err != nil {
		return "", err
	}
	port := int(*port32)
	outText, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command("env", fmt.Sprintf("PGPASSWORD=%s", password),
		"psql", "--host=localhost", fmt.Sprintf("--port=%d", port), fmt.Sprintf("--user=%s", username), dbName, fmt.Sprintf("--command=%s", sqlCommand)))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(outText), nil
}

func (f *Framework) waitUntilPatchedConfigMapReady(meta metav1.ObjectMeta) error {
	pod, _, _, err := f.getPodPassPort(meta)
	if err != nil {
		return err
	}

	return wait.PollImmediate(time.Second, kutil.ReadinessTimeout, func() (bool, error) {
		outText, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command("cat", "/var/run/pgbouncer/pb_status"))
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(outText) == "RELOADED" {
			return true, nil
		}
		return false, nil
	})
}

// getPodPassPort returns the pgbouncer pod, pgbouncer admin pass, and pgbouncer listen port
func (f *Framework) getPodPassPort(meta metav1.ObjectMeta) (*v1.Pod, string, *int32, error) {
	pb, err := f.dbClient.KubedbV1alpha2().PgBouncers(meta.Namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
	if err != nil {
		return nil, "", nil, err
	}

	pod, err := f.kubeClient.CoreV1().Pods(meta.Namespace).Get(context.TODO(), meta.Name+"-0", metav1.GetOptions{})
	if err != nil {
		return nil, "", nil, err
	}

	pass, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command("cat", filepath.Join(controller.UserListMountPath, "pb-password")))
	if err != nil {
		return nil, "", nil, err
	}

	return pod, strings.TrimSpace(pass), pb.Spec.ConnectionPool.Port, nil
}

func (f *Framework) WaitUntilPrimaryContainerReady(meta metav1.ObjectMeta) error {
	// primary container may a while to serve responses after pulling the image
	sts, err := f.kubeClient.AppsV1().StatefulSets(f.namespace).Get(context.TODO(), meta.Name, metav1.GetOptions{})
	if err != nil {
		return err
	}

	err = wait.PollImmediate(time.Second, kutil.ReadinessTimeout, func() (bool, error) {
		for i := 0; i < int(pointer.Int32(sts.Spec.Replicas)); i++ {
			pod, err := f.kubeClient.CoreV1().Pods(meta.Namespace).Get(context.TODO(), meta.Name+"-"+strconv.Itoa(i), metav1.GetOptions{})
			if err != nil {
				if kerr.IsNotFound(err) {
					return false, nil
				} else {
					return false, err
				}
			}
			_, err = exec.ExecIntoPod(f.restConfig, pod, exec.Command("cat", filepath.Join(controller.UserListMountPath, "pb-password")))
			if err != nil {
				return false, nil
			}
		}
		return true, nil
	})
	return err
}
