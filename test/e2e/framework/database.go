package framework

import (
	"crypto/rand"
	"fmt"
	"github.com/appscode/go/log"
	"strings"
	"time"

	shell "github.com/codeskyblue/go-sh"
	"github.com/go-xorm/xorm"
	_ "github.com/lib/pq"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kutil "kmodules.xyz/client-go"
	"kmodules.xyz/client-go/tools/portforward"
	"kubedb.dev/pgbouncer/pkg/controller"
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

func (f *Framework) GetPgBouncerClient(tunnel *portforward.Tunnel, dbName string, userName string) (*xorm.Engine, error) {
	cnnstr := fmt.Sprintf("user=%s host=127.0.0.1 port=%v dbname=%s sslmode=disable", userName, tunnel.Local, dbName)
	return xorm.NewEngine("pgbouncer", cnnstr)
}

func (f *Framework) GetPostgresClient(tunnel *portforward.Tunnel, dbName string, userName string) (*xorm.Engine, error) {
	cnnstr := fmt.Sprintf("user=%s host=127.0.0.1 port=%v dbname=%s sslmode=disable", userName, tunnel.Local, dbName)
	return xorm.NewEngine("postgres", cnnstr)
}

func (f *Framework) EventuallyCreateSchema(meta metav1.ObjectMeta, dbName string, userName string) GomegaAsyncAssertion {
	sql := fmt.Sprintf(`
DROP SCHEMA IF EXISTS "data" CASCADE;
CREATE SCHEMA "data" AUTHORIZATION "%s";`, userName)
	return Eventually(
		func() bool {
			tunnel, err := f.ForwardPort(meta)
			if err != nil {
				return false
			}
			defer tunnel.Close()

			db, err := f.GetPgBouncerClient(tunnel, dbName, userName)
			if err != nil {
				return false
			}
			defer db.Close()

			if err := f.CheckPgBouncer(db); err != nil {
				return false
			}

			_, err = db.Exec(sql)
			if err != nil {
				return false
			}
			return true
		},
		time.Minute*5,
		time.Second*5,
	)
}

var randChars = []rune("abcdefghijklmnopqrstuvwxyzabcdef")

// Use this for generating random pat of a ID. Do not use this for generating short passwords or secrets.
func characters(len int) string {
	bytes := make([]byte, len)
	rand.Read(bytes)
	r := make([]rune, len)
	for i, b := range bytes {
		r[i] = randChars[b>>3]
	}
	return string(r)
}

func (f *Framework) EventuallyPingDatabase(meta metav1.ObjectMeta, dbName string, userName string) GomegaAsyncAssertion {
	println(dbName, "::::::::::::", userName)
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

func (f *Framework) EventuallyPingPgBouncer(meta metav1.ObjectMeta) error {
	return wait.PollImmediate(operatorGetRetryInterval, kutil.ReadinessTimeout, func() (bool, error) {
		println("Printing ping function")
		tunnel, err := f.ForwardPgBouncerPort(meta)
		if err != nil {
			println("Err = ", err)
			return false, nil
		}
		defer tunnel.Close()
		println("Local tunnel = ", tunnel.Local)
		pingResult := f.PingPgBouncer(tunnel.Local)
		println("::::Ping result = ", pingResult)
		return pingResult, nil
	})
}

func (f *Framework) WaitToPingPgBouncer(meta metav1.ObjectMeta) GomegaAsyncAssertion {
	return Eventually(
		func() bool {
			println("Printing ping function")
			tunnel, err := f.ForwardPgBouncerPort(meta)
			if err != nil {
				return false
			}
			defer tunnel.Close()
			println("LOcal tunnel = ", tunnel.Local)
			pingResult := f.PingPgBouncer(tunnel.Local)
			println("::::Ping result = ", pingResult)
			return pingResult
		},
		time.Minute*10,
		time.Second*5,
	)
}

func (f *Framework) PingPgBouncer(port int) bool {
	println("PING PING")
	sh := shell.NewSession()
	cmd := sh.Command("env","PGPASSWORD=kubedb123", "psql",
		"--host=localhost", fmt.Sprintf("--port=%d", port),
		fmt.Sprintf("--username=%s", PgBouncerAdmin), PgBouncerAdmin, "--command=RELOAD")
	out, err := cmd.Output()
	if err != nil {
		log.Infoln("CMD out err = ", err)
		return false
	}
	outText := strings.TrimSpace(string(out))
	if outText != CmdReload{
		return false
	}
	return true
}

func (f *Framework) EventuallyCreateTable(meta metav1.ObjectMeta, dbName string, userName string, total int) GomegaAsyncAssertion {
	count := 0
	return Eventually(
		func() bool {
			tunnel, err := f.ForwardPort(meta)
			if err != nil {
				return false
			}
			defer tunnel.Close()

			db, err := f.GetPgBouncerClient(tunnel, dbName, userName)
			if err != nil {
				return false
			}
			defer db.Close()

			if err := f.CheckPgBouncer(db); err != nil {
				return false
			}

			for i := count; i < total; i++ {
				table := fmt.Sprintf("SET search_path TO \"data\"; CREATE TABLE %v ( id bigserial )", characters(5))
				_, err := db.Exec(table)
				if err != nil {
					return false
				}
				count++
			}
			return true
		},
		time.Minute*5,
		time.Second*5,
	)

	return nil
}

func (f *Framework) EventuallyCountTable(meta metav1.ObjectMeta, dbName string, userName string) GomegaAsyncAssertion {
	return Eventually(
		func() int {
			tunnel, err := f.ForwardPort(meta)
			if err != nil {
				return -1
			}
			defer tunnel.Close()

			db, err := f.GetPgBouncerClient(tunnel, dbName, userName)
			if err != nil {
				return -1
			}
			defer db.Close()

			if err := f.CheckPgBouncer(db); err != nil {
				return -1
			}

			res, err := db.Query("SELECT table_name FROM information_schema.tables WHERE table_schema='data'")
			if err != nil {
				return -1
			}

			return len(res)
		},
		time.Minute*10,
		time.Second*5,
	)
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

func (f *Framework) EventuallyCountArchive(meta metav1.ObjectMeta, dbName string, userName string) GomegaAsyncAssertion {
	previousCount := -1
	countSet := false
	return Eventually(
		func() bool {
			tunnel, err := f.ForwardPort(meta)
			if err != nil {
				return false
			}
			defer tunnel.Close()

			db, err := f.GetPgBouncerClient(tunnel, dbName, userName)
			if err != nil {
				return false
			}
			defer db.Close()

			if err := f.CheckPgBouncer(db); err != nil {
				return false
			}

			var archiver PgStatArchiver
			if _, err := db.Limit(1).Cols("archived_count").Get(&archiver); err != nil {
				return false
			}

			if !countSet {
				countSet = true
				previousCount = archiver.ArchivedCount
				return false
			} else {
				if archiver.ArchivedCount > previousCount {
					return true
				}
			}
			return false
		},
		time.Minute*10,
		time.Second*5,
	)
}

func (f *Framework) EventuallyPGSettings(meta metav1.ObjectMeta, dbName string, userName string, config string) GomegaAsyncAssertion {
	configPair := strings.Split(config, "=")
	sql := fmt.Sprintf("SHOW %s;", configPair[0])
	return Eventually(
		func() []map[string][]byte {
			tunnel, err := f.ForwardPort(meta)
			if err != nil {
				return nil
			}
			defer tunnel.Close()

			db, err := f.GetPgBouncerClient(tunnel, dbName, userName)
			if err != nil {
				return nil
			}
			defer db.Close()

			if err := f.CheckPgBouncer(db); err != nil {
				return nil
			}

			results, err := db.Query(sql)
			if err != nil {
				return nil
			}
			return results
		},
		time.Minute*5,
		time.Second*5,
	)
}
