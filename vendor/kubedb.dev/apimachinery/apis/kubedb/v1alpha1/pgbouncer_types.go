package v1alpha1

import (
	"github.com/appscode/go/encoding/json/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	mona "kmodules.xyz/monitoring-agent-api/api/v1"
	v1 "kmodules.xyz/offshoot-api/api/v1"
)

const (
	ResourceCodePgBouncer     = "pb"
	ResourceKindPgBouncer     = "PgBouncer"
	ResourceSingularPgBouncer = "pgbouncer"
	ResourcePluralPgBouncer   = "pgbouncers"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PgBouncer defines a PgBouncer database.
type PgBouncer struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PgBouncerSpec   `json:"spec,omitempty"`
	Status            PgBouncerStatus `json:"status,omitempty"`
}

type PgBouncerSpec struct {
	// Version of PgBouncer to be deployed.
	Version types.StrYo `json:"version"`
	// Number of instances to deploy for a PgBouncer instance.
	Replicas *int32 `json:"replicas,omitempty"`
	// ServiceTemplate is an optional configuration for service used to expose database
	// +optional
	ServiceTemplate v1.ServiceTemplateSpec `json:"serviceTemplate,omitempty"`
	// Databases to proxy by connection pooling
	// +optional
	Databases []Databases `json:"databases, omitempty"`
	// ConnectionPoolConfig defines Connection pool configuration
	ConnectionPool *ConnectionPoolConfig `json:"connectionPool"`
	// UserList keeps a list of pgbouncer user's secrets
	// +optional
	UserList UserList `json:"userList, omitempty"`
	// Monitor is used monitor database instance
	// +optional
	Monitor *mona.AgentSpec `json:"monitor,omitempty"`
}

type Databases struct {
	//alias to uniquely identify a target database running inside a specific Postgres instance
	Alias             string `json:"alias"`
	//Name of the target database inside a Postgres instance
	DbName            string `json:"databaseName"`
	//Reference to Postgres instance where the target database is located
	AppBindingName      string `json:"appBindingName"`
	//Namespace of PgBouncer object
	//if left empty, pgBouncer namespace is assigned
	// use "default" for dafault namespace
	// +optional
	AppBindingNamespace string `json:"appBindingNamespace,omitempty"`
	//To bind a single user to a specific connection
	// +optional
	UserName        string `json:"username,omitempty"`
}

type ConnectionPoolConfig struct {
	ListenPort    *int32   `json:"listenPort"`
	ListenAddress string   `json:"listenAddress,omitempty"`
	PoolMode      string   `json:"poolMode,omitempty"`
	AdminUsers    []string `json:"adminUsers,omitempty"`
	// observedGeneration is the most recent generation observed for this resource. It corresponds to the
	// resource's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration *types.IntHash `json:"observedGeneration,omitempty"`
}

type UserList struct {
	SecretName      string `json:"name"`                //contains a single username-password combo that exists in a target database
	SecretNamespace string `json:"namespace,omitempty"` //Namespace of PgBouncer object
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type PgBouncerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	// Items is a list of PgBouncer CRD objects
	Items []PgBouncer `json:"items,omitempty"`
}

// Following structures are used for audit summary report
type PgBouncerTableInfo struct {
	TotalRow int64 `json:"totalRow"`
	MaxID    int64 `json:"maxId"`
	NextID   int64 `json:"nextId"`
}

type PgBouncerSchemaInfo struct {
	Table map[string]*PgBouncerTableInfo `json:"table"`
}

type PgBouncerSummary struct {
	Schema map[string]*PgBouncerSchemaInfo `json:"schema"`
}

type PgBouncerStatus struct {
	Phase  DatabasePhase `json:"phase,omitempty"`
	Reason string        `json:"reason,omitempty"`
	// observedGeneration is the most recent generation observed for this resource. It corresponds to the
	// resource's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration *types.IntHash `json:"observedGeneration,omitempty"`
}
