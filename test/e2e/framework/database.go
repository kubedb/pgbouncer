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
package framework

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/go-xorm/xorm"
	_ "github.com/lib/pq"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kutil "kmodules.xyz/client-go"
	"kmodules.xyz/client-go/tools/exec"
	"kmodules.xyz/client-go/tools/portforward"
	appcat "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
	"kubedb.dev/pgbouncer/pkg/controller"
)

const (
	testUser = "myuser"
	testPass = "mypass"
	testDB   = "tmpdb"
)

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
		return f.PingPgBouncerServer(meta)
	})
}

func (f *Framework) PingPgBouncerServer(meta metav1.ObjectMeta) (bool, error) {
	pod, password, port32, err := f.getPodPassPort(meta)
	if err != nil {
		return false, err
	}
	port := int(*port32)

	pgbouncer := api.ResourceSingularPgBouncer
	outText, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command("env", fmt.Sprintf("PGPASSWORD=%s", password),
		"psql", "--host=localhost", fmt.Sprintf("--port=%d", port), "--user=kubedb", pgbouncer, "--command=RELOAD"))
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(outText) == CmdReload, nil
}

func (f *Framework) CheckPostgres(db *xorm.Engine) error {
	return db.Ping()
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

	err = f.CreateTableViaPgBouncer(meta, testUser, testPass, testDB)
	if err != nil {
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
	pod,_,port32, err := f.getPodPassPort(meta)
	if err != nil {
		return "", err
	}
	port := int(*port32)

	outText, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command("env", fmt.Sprintf("PGPASSWORD=%s", password),
		"psql", "--host=localhost", fmt.Sprintf("--port=%d", port), fmt.Sprintf("--user=%s", username), dbName, fmt.Sprintf("--command=%s", sqlCommand)))
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(outText)), nil
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

func (f *Framework) getPodPassPort(meta metav1.ObjectMeta) (*v1.Pod, string, *int32, error) {
	//returns the pgbouncer pod, pgbouncer admin pass, and pgbouncer listen port
	pb, err := f.dbClient.KubedbV1alpha1().PgBouncers(meta.Namespace).Get(meta.Name, metav1.GetOptions{})
	if err != nil {
		return nil, "", nil, err
	}

	pod, err := f.kubeClient.CoreV1().Pods(meta.Namespace).Get(meta.Name+"-0", metav1.GetOptions{})
	if err != nil {
		return nil, "", nil, err
	}
	pass, err := exec.ExecIntoPod(f.restConfig, pod, exec.Command("cat", "/var/run/pgbouncer/secret/pb-password"))
	if err != nil {
		return nil, "", nil, err
	}
	return pod, strings.TrimSpace(pass), pb.Spec.ConnectionPool.Port, nil

}
