package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SVMSpec defines the desired state of SVM
type SVMSpec struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	ControllerUrl     string                      `json: "controllerUrl"`
	AccountName       string                      `json: "accountName"`
	GlobalAccountName string                      `json: "globalAccountName"`
	IncludeBiq        bool                        `json: "includeBiq,omitempty"`
	Debug             bool                        `json: "debug,omitempty"`
	SecretName        string                      `json: "secretName"`
	Image             string                      `json: "image,omitempty"`
	Args              []string                    `json: "args,omitempty"`
	Env               []corev1.EnvVar             `json: "env,omitempty"`
	Resources         corev1.ResourceRequirements `json: "resources,omitempty"`
	NodeSelector      map[string]string           `json: "nodeSelector,omitempty"`
	Tolerations       []corev1.Toleration         `json: "tolerations,omitempty"`
}

// SVMStatus defines the observed state of SVM
type SVMStatus struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	AgentVersion     string            `json:"version,omitempty"`
	Items            map[string]string `json:"agentVersion,omitempty"`
	UpdatedTimestamp metav1.Time       `json:"updatedTimestamp,omitempty"`
}

//// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
//type SVMInstance struct {
//	PodName      string `json:"podName,omitempty"`
//	AgentVersion string `json:"agentVersion,omitempty"`
//}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SVM is the Schema for the svms API
// +k8s:openapi-gen=true
type SVM struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SVMSpec   `json:"spec,omitempty"`
	Status SVMStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SVMList contains a list of SVM
type SVMList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SVM `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SVM{}, &SVMList{})
}
