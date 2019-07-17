package v1alpha1

import (
	"github.com/appscode/go/encoding/json/types"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
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
	//Version string `json:"version"`

	// Number of instances to deploy for a PgBouncer instance.
	Replicas *int32 `json:"replicas,omitempty"`

	ServiceTemplate v1.ServiceTemplateSpec `json:"serviceTemplate,omitempty"`

	// Databases to proxy
	Databases []Databases `json:"databases"`

	// Connection pool configuration
	ConnectionPoolConfig *ConnectionPoolConfig `json:"connectionPoolConfig"`
	//Pointer?

	//list of secrets
	SecretList []SecretList `json:"secretList"`
	// Image is used for local only machines
	// +optional
	Image string `json:"image,omitempty"`
	// Monitor is used monitor database instance
	// +optional
	Monitor *mona.AgentSpec `json:"monitor,omitempty"`

	// ConfigSource is an optional field to provide custom configuration file for database (i.e pgbouncerql.conf).
	// If specified, this file will be used as configuration file otherwise default configuration file will be used.
	ConfigSource *core.VolumeSource `json:"configSource,omitempty"`

	// updateStrategy indicates the StatefulSetUpdateStrategy that will be
	// employed to update Pods in the StatefulSet when a revision is made to
	// Template.
	UpdateStrategy apps.StatefulSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// TerminationPolicy controls the delete operation for database
	// +optional
	TerminationPolicy TerminationPolicy `json:"terminationPolicy,omitempty"`
}

type Databases struct {
	Alias             string `json:"dbAlias"`                     //alias to identify target database
	DbName            string `json:"dbName"`                      //Name of the target database
	PgObjectName      string `json:"pgObjectName"`                //PgBouncer object where the target database is located
	PgObjectNamespace string `json:"pgObjectNamespace,omitempty"` //Namespace of PgBouncer object
	SecretName        string `json:"secretName,omitempty"`        //To bind a single user to a specific connection , Optional
}

type ConnectionPoolConfig struct {
	ListenPort    string   `json:"listenPort"`
	ListenAddress string   `json:"listenAddress,omitempty"`
	PoolMode      string   `json:"poolMode,omitempty"`
	AdminUsers    []string `json:"adminUsers,omitempty"`
	// observedGeneration is the most recent generation observed for this resource. It corresponds to the
	// resource's generation, which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration *types.IntHash `json:"observedGeneration,omitempty"`
}

type SecretList struct {
	SecretName      string `json:"secretName"`                //contains a single username-password combo that exists in a target database
	SecretNamespace string `json:"secretNamespace,omitempty"` //Namespace of PgBouncer object
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
