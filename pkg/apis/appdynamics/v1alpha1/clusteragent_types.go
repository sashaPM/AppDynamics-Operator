package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterAgentSpec defines the desired state of ClusterAgent
type ClusterAgentSpec struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	ControllerUrl     string `json: "controllerUrl"`
	AccountName       string `json: "accountName"`
	GlobalAccountName string `json: "globalAccountName"`
	//reference to the secret with credentials
	//controller-key
	//events-api-key
	//rest-user
	SecretName string                      `json: "secretName"`
	Image      string                      `json:"image,omitempty"`
	Args       []string                    `json:"args,omitempty"`
	Env        []corev1.EnvVar             `json:"env,omitempty"`
	Resources  corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ClusterAgentStatus defines the observed state of ClusterAgent
type ClusterAgentStatus struct {
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	LastMetricsUpdateTime metav1.Time `json:"lastMetricsUpdateTime"`
	LastChangePushTime    metav1.Time `json:"lastChangePushTime"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterAgent is the Schema for the clusteragents API
// +k8s:openapi-gen=true
type ClusterAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterAgentSpec   `json:"spec,omitempty"`
	Status ClusterAgentStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterAgentList contains a list of ClusterAgent
type ClusterAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterAgent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterAgent{}, &ClusterAgentList{})
}
