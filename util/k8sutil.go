package util

import (
	apps_v1 "k8s.io/api/apps/v1"
	batch_v1 "k8s.io/api/batch/v1"
	core_v1 "k8s.io/api/core/v1"
	net_beta1 "k8s.io/api/networking/v1beta1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GetObjectMetadata(obj interface{}) (objectMeta meta_v1.ObjectMeta) {
	switch object := obj.(type) {
	case apps_v1.Deployment:
		objectMeta = object.ObjectMeta
	case apps_v1.DaemonSet:
		objectMeta = object.ObjectMeta
	case apps_v1.ReplicaSet:
		objectMeta = object.ObjectMeta
	case core_v1.Service:
		objectMeta = object.ObjectMeta
	case core_v1.Node:
		objectMeta = object.ObjectMeta
	case core_v1.Pod:
		objectMeta = object.ObjectMeta
	case core_v1.ServiceAccount:
		objectMeta = object.ObjectMeta
	case core_v1.ConfigMap:
		objectMeta = object.ObjectMeta
	case core_v1.Event:
		objectMeta = object.ObjectMeta
	case core_v1.Secret:
		objectMeta = object.ObjectMeta
	case core_v1.PersistentVolume:
		objectMeta = object.ObjectMeta
	case core_v1.Namespace:
		objectMeta = object.ObjectMeta
	case batch_v1.Job:
		objectMeta = object.ObjectMeta
	case net_beta1.Ingress:
		objectMeta = object.ObjectMeta
	}

	return objectMeta
}
