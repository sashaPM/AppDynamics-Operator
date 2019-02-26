package clusteragent

import (
	"bytes"
	"context"
	"fmt"

	"encoding/json"

	"strconv"
	"strings"

	appdynamicsv1alpha1 "github.com/sjeltuhin/appdynamics-operator/pkg/apis/appdynamics/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_clusteragent")

const (
	AGENT_SECRET_NAME string = "cluster-agent-secret"
	AGENt_CONFIG_NAME string = "cluster-agent-config"
)

/**
* USER ACTION REQUIRED: This is a scaffold file intended for the user to modify with their own Controller
* business logic.  Delete these comments after modifying this file.*
 */

// Add creates a new ClusterAgent Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileClusterAgent{client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("clusteragent-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ClusterAgent
	err = c.Watch(&source.Kind{Type: &appdynamicsv1alpha1.ClusterAgent{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// TODO(user): Modify this to be the types you create that are owned by the primary resource
	// Watch for changes to secondary resource Deployment and requeue the owner ClusterAgent
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &appdynamicsv1alpha1.ClusterAgent{},
	})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileClusterAgent{}

// ReconcileClusterAgent reconciles a ClusterAgent object
type ReconcileClusterAgent struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a ClusterAgent object and makes changes based on the state read
// and what is in the ClusterAgent.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileClusterAgent) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ClusterAgent")

	// Fetch the ClusterAgent instance
	clusterAgent := &appdynamicsv1alpha1.ClusterAgent{}
	err := r.client.Get(context.TODO(), request.NamespacedName, clusterAgent)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Cluster Agent resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get Cluster Agent")
		return reconcile.Result{}, err
	}

	// Check if the agent already exists in the namespace
	existingDeployment := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: clusterAgent.Name, Namespace: clusterAgent.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		_, esecret := r.ensureSecret(clusterAgent)
		if esecret != nil {
			reqLogger.Error(esecret, "Failed to create new Cluster Agent due to secret", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
			return reconcile.Result{}, esecret
		}
		_, econfig := r.ensureConfigMap(clusterAgent)
		if econfig != nil {
			reqLogger.Error(econfig, "Failed to create new Cluster Agent due to config map", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
			return reconcile.Result{}, econfig
		}
		_, esvc := r.ensureAgentService(clusterAgent)
		if esvc != nil {
			reqLogger.Error(esvc, "Failed to create new Cluster Agent due to service", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
			return reconcile.Result{}, esvc
		}
		// Define a new deployment for the cluster agent
		dep := r.newAgentDeployment(clusterAgent)
		reqLogger.Info("Creating a new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		err = r.client.Create(context.TODO(), dep)
		if err != nil {
			reqLogger.Error(err, "Failed to create new Deployment", "Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return reconcile.Result{}, err
		}
		reqLogger.Info("Deployment created successfully")
		// Deployment created successfully - don't requeue
		return reconcile.Result{}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Deployment")
		return reconcile.Result{}, err
	}

	// Ensure the deployment spec matches the new spec
	// Differentiate between breaking changes and benign updates

	//	if *found.Spec.ControllerUrl != clusterAgent.Spec.ControllerUrl {
	//		found.Spec.ControllerUrl = &clusterAgent.Spec.ControllerUrl
	//		err = r.client.Update(context.TODO(), found)
	//		if err != nil {
	//			reqLogger.Error(err, "Failed to update Deployment", "Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)
	//			return reconcile.Result{}, err
	//		}
	//		// Spec updated - return and requeue
	//		return reconcile.Result{Requeue: true}, nil
	//	}

	//	// Update the ClusterAgent status with the pod names
	//	// List the pods for this memcached's deployment
	//	podList := &corev1.PodList{}
	//	labelSelector := labels.SelectorFromSet(labelsForClusterAgent(clusterAgent.Name))
	//	listOps := &client.ListOptions{Namespace: clusterAgent.Namespace, LabelSelector: labelSelector}
	//	err = r.client.List(context.TODO(), listOps, podList)
	//	if err != nil {
	//		reqLogger.Error(err, "Failed to list pods", "clusterAgent.Namespace", clusterAgent.Namespace, "clusterAgent.Name", clusterAgent.Name)
	//		return reconcile.Result{}, err
	//	}
	//	podNames := getPodNames(podList.Items)

	//	// Update status.Nodes if needed
	//	if !reflect.DeepEqual(podNames, memcached.Status.Nodes) {
	//		clusterAgent.Status.Nodes = podNames
	//		err := r.client.Status().Update(context.TODO(), clusterAgent)
	//		if err != nil {
	//			reqLogger.Error(err, "Failed to update Memcached status")
	//			return reconcile.Result{}, err
	//		}
	//	}
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterAgent) ensureSecret(clusterAgent *appdynamicsv1alpha1.ClusterAgent) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	key := client.ObjectKey{Namespace: clusterAgent.Namespace, Name: "cluster-agent-secret"}
	err := r.client.Get(context.TODO(), key, secret)
	if err != nil {
		return nil, fmt.Errorf("Unable to get secret for cluster-agent. %v", err)
	}
	return secret, nil
}

func (r *ReconcileClusterAgent) ensureAgentService(clusterAgent *appdynamicsv1alpha1.ClusterAgent) (*corev1.Service, error) {
	selector := labelsForClusterAgent(clusterAgent)
	svc := &corev1.Service{}
	key := client.ObjectKey{Namespace: clusterAgent.Namespace, Name: "cluster-agent-service"}
	err := r.client.Get(context.TODO(), key, svc)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("Unable to get service for cluster-agent. %v", err)
	}

	if err != nil && errors.IsNotFound(err) {
		svc := &corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Service",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterAgent.Name,
				Namespace: clusterAgent.Namespace,
				Labels:    selector,
			},
			Spec: corev1.ServiceSpec{
				Selector: selector,
				Ports: []corev1.ServicePort{
					{
						Name:     "cluster-agent-listener",
						Protocol: corev1.ProtocolTCP,
						Port:     8989,
					},
				},
			},
		}
		err = r.client.Create(context.TODO(), svc)
		if err != nil {
			return nil, fmt.Errorf("Failed to create cluster agent service: %v", err)
		}
	}
	return svc, nil
}

func (r *ReconcileClusterAgent) ensureConfigMap(clusterAgent *appdynamicsv1alpha1.ClusterAgent) (*corev1.ConfigMap, error) {
	cm := &corev1.ConfigMap{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: "cluster-agent-config", Namespace: clusterAgent.Namespace}, cm)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("Failed to load configMap cluster-agent-config. %v", err)
	}
	if err != nil && errors.IsNotFound(err) {
		//configMap does not exist. Create
		cm.Name = "cluster-agent-config"
		cm.Namespace = clusterAgent.Namespace
		bag := AppDBag{}
		bag.Account = clusterAgent.Spec.AccountName
		bag.GlobalAccount = clusterAgent.Spec.GlobalAccountName
		arr := strings.Split(clusterAgent.Spec.ControllerUrl, ":")
		if len(arr) != 3 {
			return nil, fmt.Errorf("Enable to create configMap. Controller Url is invalid. Use this format: protocol://url:port")
		}
		protocol := arr[0]
		controllerUrl := strings.TrimLeft(arr[1], "//")
		port, errPort := strconv.Atoi(arr[2])
		if errPort != nil {
			return nil, fmt.Errorf("Enable to create configMap. Controller port is invalid. %v", errPort)
		}
		bag.ControllerUrl = controllerUrl
		bag.ControllerPort = uint16(port)
		bag.SSLEnabled = strings.Contains(protocol, "s")

		data, errJson := json.Marshal(bag)
		if errJson != nil {
			return nil, fmt.Errorf("Enable to create configMap. Cannot serialize the config Bag. %v", errJson)
		}
		cm.Data["cluster-agent-config.yaml"] = string(data)
		e := r.client.Update(context.TODO(), cm)
		if e != nil {
			return nil, fmt.Errorf("Failed to save configMap cluster-agent-config. %v", e)
		}
	}

	return cm, nil

}

func (r *ReconcileClusterAgent) newAgentDeployment(clusterAgent *appdynamicsv1alpha1.ClusterAgent) *appsv1.Deployment {
	ls := labelsForClusterAgent(clusterAgent)
	var replicas int32 = 1
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterAgent.Name,
			Namespace: clusterAgent.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: ls,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "appd-operator",
					Containers: []corev1.Container{{
						Env: []corev1.EnvVar{
							{
								Name: "ACCESS_KEY",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: AGENT_SECRET_NAME},
										Key:                  "controller-key",
									},
								},
							},
							{
								Name: "EVENT_ACCESS_KEY",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: AGENT_SECRET_NAME},
										Key:                  "event-key",
									},
								},
							},
							{
								Name: "REST_API_CREDENTIALS",
								ValueFrom: &corev1.EnvVarSource{
									SecretKeyRef: &corev1.SecretKeySelector{
										LocalObjectReference: corev1.LocalObjectReference{Name: AGENT_SECRET_NAME},
										Key:                  "api-user",
									},
								},
							},
						},
						Image:           clusterAgent.Spec.Image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Name:            "cluster-agent",
						Resources:       clusterAgent.Spec.Resources,
						Ports: []corev1.ContainerPort{{
							ContainerPort: 8989,
							Name:          "cluster-agent-listener",
						}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "agent-config",
							MountPath: "/opt/appdynamics/config/cluster-agent-config.yaml",
						}},
					}},
					Volumes: []corev1.Volume{{
						Name: "agent-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "cluster-agent-config",
								},
							},
						},
					}},
				},
			},
		},
	}
	// Set Cluster Agent instance as the owner and controller
	controllerutil.SetControllerReference(clusterAgent, dep, r.scheme)
	return dep
}

func labelsForClusterAgent(clusterAgent *appdynamicsv1alpha1.ClusterAgent) map[string]string {
	return map[string]string{"app": "clusterAgent", "clusterAgent_cr": clusterAgent.Name}
}

func getConfigMap(data string, clusterAgent *appdynamicsv1alpha1.ClusterAgent) string {
	buf := bytes.NewBufferString(data)
	buf.WriteString(
		`Account: ""
	GlobalAccount: ""
	ControllerUrl:
	ControllerPort:
	EventServiceUrl:
	SSLEnabled:
    AppName: "K8s-Cluster-Agent"
	TierName: "ClusterAgent"
	NodeName: "Node1"
	SystemSSLCert: "/opt/appd/ssl/system.crt"
	AgentSSLCert: "/opt/appd/ssl/agent.crt"
	EventAPILimit: 100
	PodSchemaName: "schema-pods"
	NodeSchemaName: "schema-nodes"
	EventSchemaName: "schema-events"
	ContainerSchemaName: "schema-containers"
	JobSchemaName: "schema-jobs"
	LogSchemaName: "schema-logs"
	DashboardTemplatePath: "/usr/local/go/src/github.com/sjeltuhin/clusterAgent/templates/k8s_dashboard_template.json"
	DashboardSuffix: "SUMMARY"
	JavaAgentVersion: "latest"
	AgentLabel: "appd-agent"
	AppDAppLabel: "appd-app"
	AppDTierLabel: "appd-tier"
	AppDAnalyticsLabel: "appd-biq"
	AgentMountName: "appd-agent-repo"
	AgentMountPath: "/opt/appd"
	AppLogMountName: "appd-volume"
	AppLogMountPath: "/opt/appdlogs"
	JDKMountName: "jdk-repo"
	JDKMountPath: "$JAVA_HOME/lib"
	NodeNamePrefix: ""
	AnalyticsAgentUrl: "http://analytics-proxy:9090"
	AnalyticsAgentContainerName: "appd-analytics-agent"
	AppDInitContainerName" appd-agent-attach"
	AnalyticsAgentImage: "sashaz/analytics-agent@sha256:ff776bdf3beed9f4bdf638d16b5a688d9e1c0fc124ce1282bef1851c122397e4"
	AppDJavaAttachImage: "sashaz/java-agent-attach@sha256:b93f2018b091f4abfd2533e6c194c9e6ecf00fcae861c732f1b771dad1b26a80"
	AppDDotNetAttachImage: "sashaz/dotnet-agent-attach@sha256:3f5d921eadfa227ffe072caa41e01c3c1fc882c5617ad45d808ffedaa20593a6"
	AppDNodeJSAttachImage: "latest"
	ProxyInfo: ""
	ProxyUser: ""
	ProxyPass: ""
	InstrumentationMethod: "mount"
	InitContainerDir: "/opt/temp."
	MetricsSyncInterval: 60
	SnapshotSyncInterval: 15
	AgentServerPort: 8989
	IncludeNsToInstrument: []
	ExcludeNsToInstrument: []
	DeploysToDashboard: ["client-api"]
	IncludeNodesToInstrument: []
	ExcludeNodesToInstrument: []`)

	return buf.String()
}
