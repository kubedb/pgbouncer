/*
Copyright 2019 The KubeDB Authors.

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

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	time "time"

	catalogv1alpha1 "github.com/kubedb/apimachinery/apis/catalog/v1alpha1"
	versioned "github.com/kubedb/apimachinery/client/clientset/versioned"
	internalinterfaces "github.com/kubedb/apimachinery/client/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/kubedb/apimachinery/client/listers/catalog/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// MySQLVersionInformer provides access to a shared informer and lister for
// MySQLVersions.
type MySQLVersionInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.MySQLVersionLister
}

type mySQLVersionInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
}

// NewMySQLVersionInformer constructs a new informer for MySQLVersion type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewMySQLVersionInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredMySQLVersionInformer(client, resyncPeriod, indexers, nil)
}

// NewFilteredMySQLVersionInformer constructs a new informer for MySQLVersion type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredMySQLVersionInformer(client versioned.Interface, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.CatalogV1alpha1().MySQLVersions().List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.CatalogV1alpha1().MySQLVersions().Watch(options)
			},
		},
		&catalogv1alpha1.MySQLVersion{},
		resyncPeriod,
		indexers,
	)
}

func (f *mySQLVersionInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredMySQLVersionInformer(client, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *mySQLVersionInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&catalogv1alpha1.MySQLVersion{}, f.defaultInformer)
}

func (f *mySQLVersionInformer) Lister() v1alpha1.MySQLVersionLister {
	return v1alpha1.NewMySQLVersionLister(f.Informer().GetIndexer())
}
