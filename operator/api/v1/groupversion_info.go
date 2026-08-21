// Package v1 的 GroupVersion 注册（手写等价 kubebuilder 生成结构）。
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion 是 SLO CRD 的 group/version。
	GroupVersion = schema.GroupVersion{Group: "slo.sre-demo.io", Version: "v1"}

	// SchemeBuilder 收集 add-to-scheme 函数。
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme 注册本组类型到 scheme。
	AddToScheme = SchemeBuilder.AddToScheme
)

// addKnownTypes 把 SLO/SLOList 注册到 scheme。
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion, &SLO{}, &SLOList{})
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}
