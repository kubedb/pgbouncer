package controller

//func (c *Controller) ensureAppBinding(db *api.PgBouncer) (kutil.VerbType, error) {
//	appmeta := db.AppBindingMeta()
//
//	meta := metav1.ObjectMeta{
//		Name:      appmeta.Name(),
//		Namespace: db.Namespace,
//	}
//
//	ref, err := reference.GetReference(clientsetscheme.Scheme, db)
//	if err != nil {
//		return kutil.VerbUnchanged, err
//	}
//
//	_, vt, err := appcat_util.CreateOrPatchAppBinding(c.AppCatalogClient, meta, func(in *appcat.AppBinding) *appcat.AppBinding {
//		core_util.EnsureOwnerReference(&in.ObjectMeta, ref)
//		in.Labels = db.OffshootLabels()
//		in.Annotations = db.Spec.ServiceTemplate.Annotations
//
//		in.Spec.Type = appmeta.Type()
//		in.Spec.ClientConfig.InsecureSkipTLSVerify = false
//		in.Spec.SecretTransforms = []appcat.SecretTransform{
//			{
//				RenameKey: &appcat.RenameKeyTransform{
//					From: PgBouncerUser,
//					To:   appcat.KeyUsername,
//				},
//			},
//			{
//				RenameKey: &appcat.RenameKeyTransform{
//					From: PgBouncerPassword,
//					To:   appcat.KeyPassword,
//				},
//			},
//		}
//		return in
//	})
//
//	if err != nil {
//		return kutil.VerbUnchanged, err
//	} else if vt != kutil.VerbUnchanged {
//		c.recorder.Eventf(
//			db,
//			core.EventTypeNormal,
//			eventer.EventReasonSuccessful,
//			"Successfully %s appbinding",
//			vt,
//		)
//	}
//	return vt, nil
//}
