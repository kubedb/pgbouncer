package main

import (
	"fmt"
	"log"
	"os"

	"kubedb.dev/pgbouncer/pkg/cmds"

	"github.com/appscode/go/runtime"
	"github.com/spf13/cobra/doc"
	runtime2 "k8s.io/apimachinery/pkg/util/runtime"
)

// ref: https://github.com/spf13/cobra/blob/master/doc/md_docs.md
func main() {
	rootCmd := cmds.NewRootCmd("")
	dir := runtime.GOPath() + "/src/kubedb.dev/pgbouncer/docs/reference"
	fmt.Printf("Generating cli markdown tree in: %v\n", dir)
	err := os.RemoveAll(dir)
	if err != nil {
		log.Fatal(err)
	}
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		log.Fatal(err)
	}
	runtime2.Must(doc.GenMarkdownTree(rootCmd, dir))
}
