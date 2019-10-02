package controller

import (
	"testing"

	api "kubedb.dev/apimachinery/apis/kubedb/v1alpha1"
)

func TestConfigGeneration(t *testing.T) {
	tests := []struct {
		name      string
		pgbouncer *api.PgBouncer
	}{
		// TODO: test cases
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

		})
	}
}
