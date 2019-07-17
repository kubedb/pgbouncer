package controller

import (
	api "github.com/kubedb/apimachinery/apis/kubedb/v1alpha1"
	le "github.com/kubedb/pgbouncer/pkg/leader_election"
	apps "k8s.io/api/apps/v1"
	core "k8s.io/api/core/v1"
	policy_v1beta1 "k8s.io/api/policy/v1beta1"
	rbac "k8s.io/api/rbac/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/reference"
	core_util "kmodules.xyz/client-go/core/v1"
	rbac_util "kmodules.xyz/client-go/rbac/v1beta1"
)

func (c *Controller) ensureRole(db *api.PgBouncer, pspName string) error {
	ref, rerr := reference.GetReference(clientsetscheme.Scheme, db)
	if rerr != nil {
		return rerr
	}

	// Create new Roles
	_, _, err := rbac_util.CreateOrPatchRole(
		c.Client,
		metav1.ObjectMeta{
			Name:      db.OffshootName(),
			Namespace: db.Namespace,
		},
		func(in *rbac.Role) *rbac.Role {
			core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
			in.Labels = db.OffshootLabels()
			in.Rules = []rbac.PolicyRule{
				{
					APIGroups:     []string{apps.GroupName},
					Resources:     []string{"statefulsets"},
					Verbs:         []string{"get"},
					ResourceNames: []string{db.OffshootName()},
				},
				{
					APIGroups: []string{core.GroupName},
					Resources: []string{"pods"},
					Verbs:     []string{"list", "patch"},
				},
				{
					APIGroups: []string{core.GroupName},
					Resources: []string{"configmaps"},
					Verbs:     []string{"create"},
				},
				{
					APIGroups:     []string{core.GroupName},
					Resources:     []string{"configmaps"},
					Verbs:         []string{"get", "update"},
					ResourceNames: []string{le.GetLeaderLockName(db.OffshootName())},
				},
			}
			if pspName != "" {
				pspRule := rbac.PolicyRule{
					APIGroups:     []string{policy_v1beta1.GroupName},
					Resources:     []string{"podsecuritypolicies"},
					Verbs:         []string{"use"},
					ResourceNames: []string{pspName},
				}
				in.Rules = append(in.Rules, pspRule)
			}
			return in
		},
	)
	return err
}

func (c *Controller) ensureSnapshotRole(db *api.PgBouncer, pspName string) error {
	ref, rerr := reference.GetReference(clientsetscheme.Scheme, db)
	if rerr != nil {
		return rerr
	}
	// Create new Roles
	_, _, err := rbac_util.CreateOrPatchRole(
		c.Client,
		metav1.ObjectMeta{
			Name:      db.SnapshotSAName(),
			Namespace: db.Namespace,
		},
		func(in *rbac.Role) *rbac.Role {
			core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
			in.Labels = db.OffshootLabels()
			in.Rules = []rbac.PolicyRule{}
			if pspName != "" {
				pspRule := rbac.PolicyRule{
					APIGroups:     []string{policy_v1beta1.GroupName},
					Resources:     []string{"podsecuritypolicies"},
					Verbs:         []string{"use"},
					ResourceNames: []string{pspName},
				}
				in.Rules = append(in.Rules, pspRule)
			}
			return in
		},
	)
	return err
}

func (c *Controller) createServiceAccount(db *api.PgBouncer, saName string) error {
	ref, rerr := reference.GetReference(clientsetscheme.Scheme, db)
	if rerr != nil {
		return rerr
	}
	// Create new ServiceAccount
	_, _, err := core_util.CreateOrPatchServiceAccount(
		c.Client,
		metav1.ObjectMeta{
			Name:      saName,
			Namespace: db.Namespace,
		},
		func(in *core.ServiceAccount) *core.ServiceAccount {
			core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
			in.Labels = db.OffshootLabels()
			return in
		},
	)
	return err
}

func (c *Controller) createRoleBinding(db *api.PgBouncer, saName string) error {
	ref, rerr := reference.GetReference(clientsetscheme.Scheme, db)
	if rerr != nil {
		return rerr
	}
	// Ensure new RoleBindings
	_, _, err := rbac_util.CreateOrPatchRoleBinding(
		c.Client,
		metav1.ObjectMeta{
			Name:      db.OffshootName(),
			Namespace: db.Namespace,
		},
		func(in *rbac.RoleBinding) *rbac.RoleBinding {
			core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
			in.Labels = db.OffshootLabels()
			in.RoleRef = rbac.RoleRef{
				APIGroup: rbac.GroupName,
				Kind:     "Role",
				Name:     db.OffshootName(),
			}
			in.Subjects = []rbac.Subject{
				{
					Kind:      rbac.ServiceAccountKind,
					Name:      saName,
					Namespace: db.Namespace,
				},
			}
			return in
		},
	)
	return err
}

func (c *Controller) createSnapshotRoleBinding(db *api.PgBouncer) error {
	ref, rerr := reference.GetReference(clientsetscheme.Scheme, db)
	if rerr != nil {
		return rerr
	}
	// Ensure new RoleBindings
	_, _, err := rbac_util.CreateOrPatchRoleBinding(
		c.Client,
		metav1.ObjectMeta{
			Name:      db.SnapshotSAName(),
			Namespace: db.Namespace,
		},
		func(in *rbac.RoleBinding) *rbac.RoleBinding {
			core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
			in.Labels = db.OffshootLabels()
			in.RoleRef = rbac.RoleRef{
				APIGroup: rbac.GroupName,
				Kind:     "Role",
				Name:     db.SnapshotSAName(),
			}
			in.Subjects = []rbac.Subject{
				{
					Kind:      rbac.ServiceAccountKind,
					Name:      db.SnapshotSAName(),
					Namespace: db.Namespace,
				},
			}
			return in
		},
	)
	return err
}

func (c *Controller) getPolicyNames(db *api.PgBouncer) (string, string, error) {
	dbPolicyName := "pgbouncer"

	return dbPolicyName, "", nil
}

//
//func (c *Controller) ensureDatabaseRBAC(pgbouncer *api.PgBouncer) error {
//
//
//		saName = pgbouncer.OffshootName()
//
//
//	sa, err := c.Client.CoreV1().ServiceAccounts(pgbouncer.Namespace).Get(saName, metav1.GetOptions{})
//	if kerr.IsNotFound(err) {
//		// create service account, since it does not exist
//		if err = c.createServiceAccount(pgbouncer, saName); err != nil {
//			if !kerr.IsAlreadyExists(err) {
//				return err
//			}
//		}
//	} else if err != nil {
//		return err
//	} else if sa.Labels[meta_util.ManagedByLabelKey] != api.GenericKey {
//		// user provided the service account, so do nothing.
//		return nil
//	}
//
//	// Create New Role
//	pspName, _, err := c.getPolicyNames(pgbouncer)
//	if err != nil {
//		return err
//	}
//	if err := c.ensureRole(pgbouncer, pspName); err != nil {
//		return err
//	}
//
//	// Create New RoleBinding
//	if err := c.createRoleBinding(pgbouncer, saName); err != nil {
//		return err
//	}
//
//	return nil
//}
//
//func (c *Controller) ensureSnapshotRBAC(pgbouncer *api.PgBouncer) error {
//	_, snapshotPolicyName, err := c.getPolicyNames(pgbouncer)
//	if err != nil {
//		return err
//	}
//
//	//Role for snapshot
//	if err := c.ensureSnapshotRole(pgbouncer, snapshotPolicyName); err != nil {
//		return err
//	}
//
//	// ServiceAccount for snapshot
//	if err := c.createServiceAccount(pgbouncer, pgbouncer.SnapshotSAName()); err != nil {
//		if !kerr.IsAlreadyExists(err) {
//			return err
//		}
//	}
//
//	// Create New RoleBinding for snapshot
//	if err := c.createSnapshotRoleBinding(pgbouncer); err != nil {
//		return err
//	}
//
//	return nil
//}
