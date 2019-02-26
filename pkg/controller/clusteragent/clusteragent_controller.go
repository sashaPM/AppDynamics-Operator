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

func (r *ReconcileClusterAgent) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ClusterAgent...")

	// Fetch the ClusterAgent instance
	clusterAgent := &appdynamicsv1alpha1.ClusterAgent{}
	err := r.client.Get(context.TODO(), request.NamespacedName, clusterAgent)
	fmt.Printf("Retrieved cluster agent. Image: %s\n", clusterAgent.Spec.Image)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			reqLogger.Info("Cluster Agent resource not found. The object must be deleted")
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
		reqLogger.Info("Ensuring secret")
		_, esecret := r.ensureSecret(clusterAgent)
		if esecret != nil {
			reqLogger.Error(esecret, "Failed to create new Cluster Agent due to secret", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
			return reconcile.Result{}, esecret
		}
		reqLogger.Info("Ensuring config map")
		_, _, econfig := r.ensureConfigMap(clusterAgent)
		if econfig != nil {
			reqLogger.Error(econfig, "Failed to create new Cluster Agent due to config map", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
			return reconcile.Result{}, econfig
		}
		fmt.Printf("Creating service...\n")
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
		reqLogger.Info("Deployment created successfully. Done")
		r.updateStatus(clusterAgent)
		return reconcile.Result{}, nil
	} else if err != nil {
		reqLogger.Error(err, "Failed to get Deployment")
		return reconcile.Result{}, err
	}

	// Ensure the deployment spec matches the new spec
	// Differentiate between breaking changes and benign updates
	reqLogger.Info("Retrieving the agent config map")
	cm, bag, econfig := r.ensureConfigMap(clusterAgent)
	if econfig != nil {
		reqLogger.Error(econfig, "Failed to obtain cluster agent config map", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
		return reconcile.Result{}, econfig
	}
	breaking, benign := r.hasBreakingChanges(clusterAgent, bag)
	if breaking {
		fmt.Println("Breaking changes detected. Restarting the cluster agent pod...")
		errRestart := r.restartAgent(clusterAgent)
		if errRestart != nil {
			reqLogger.Error(errRestart, "Failed to restart cluster agent", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
			return reconcile.Result{}, errRestart
		}
	} else if benign {
		fmt.Println("Benign changes detected. Updating config map...")
		errMap := r.updateMap(cm, clusterAgent)
		if errMap != nil {
			return reconcile.Result{}, errMap
		}
		r.updateStatus(clusterAgent)
	}

	reqLogger.Info("Exiting reconciliation loop.")
	return reconcile.Result{}, nil
}

func (r *ReconcileClusterAgent) updateStatus(clusterAgent *appdynamicsv1alpha1.ClusterAgent) error {
	clusterAgent.Status.LastUpdateTime = metav1.Now()
	err := r.client.Status().Update(context.TODO(), clusterAgent)
	return err
}

func (r *ReconcileClusterAgent) hasBreakingChanges(clusterAgent *appdynamicsv1alpha1.ClusterAgent, bag *AppDBag) (bool, bool) {
	breaking := false
	benign := false

	protocol := "http://"
	if bag.SSLEnabled {
		protocol = "https://"
	}
	url := fmt.Sprintf("%s%s:%d", protocol, bag.ControllerUrl, bag.ControllerPort)
	if clusterAgent.Spec.ControllerUrl != url ||
		clusterAgent.Spec.AccountName != bag.Account ||
		clusterAgent.Spec.GlobalAccountName != bag.GlobalAccount {
		breaking = true
	}
	if !slicesEqual(clusterAgent.Spec.DashboardTiers, bag.DeploysToDashboard) {
		benign = true
	}
	return breaking, benign
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
	key := client.ObjectKey{Namespace: clusterAgent.Namespace, Name: clusterAgent.Name}
	err := r.client.Get(context.TODO(), key, svc)
	if err != nil && !errors.IsNotFound(err) {
		return nil, fmt.Errorf("Unable to get service for cluster-agent. %v\n", err)
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
						Name:     "web-port",
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

func (r *ReconcileClusterAgent) ensureConfigMap(clusterAgent *appdynamicsv1alpha1.ClusterAgent) (*corev1.ConfigMap, *AppDBag, error) {
	cm := &corev1.ConfigMap{}
	var bag AppDBag
	err := r.client.Get(context.TODO(), types.NamespacedName{Name: "cluster-agent-config", Namespace: clusterAgent.Namespace}, cm)
	if err != nil && !errors.IsNotFound(err) {
		return nil, nil, fmt.Errorf("Failed to load configMap cluster-agent-config. %v", err)
	}
	if err != nil && errors.IsNotFound(err) {
		fmt.Printf("Congigmap not found. Creating...\n")
		//configMap does not exist. Create
		cm.Name = "cluster-agent-config"
		cm.Namespace = clusterAgent.Namespace
		errMap := r.updateMap(cm, clusterAgent)
		if errMap != nil {
			return nil, nil, errMap
		}
	}
	if err == nil {
		//deserialize the map into the property bag
		jsonData := cm.Data["cluster-agent-config.json"]
		jsonErr := json.Unmarshal([]byte(jsonData), &bag)
		if jsonErr != nil {
			return nil, nil, fmt.Errorf("Enable to retrieve the configMap. Cannot deserialize. %v", jsonErr)
		}
	}

	return cm, &bag, nil

}

func (r *ReconcileClusterAgent) updateMap(cm *corev1.ConfigMap, clusterAgent *appdynamicsv1alpha1.ClusterAgent) error {
	bag := AppDBag{}
	bag.Account = clusterAgent.Spec.AccountName
	bag.GlobalAccount = clusterAgent.Spec.GlobalAccountName
	arr := strings.Split(clusterAgent.Spec.ControllerUrl, ":")
	if len(arr) != 3 {
		return fmt.Errorf("Enable to create configMap. Controller Url is invalid. Use this format: protocol://url:port")
	}
	protocol := arr[0]
	controllerUrl := strings.TrimLeft(arr[1], "//")
	port, errPort := strconv.Atoi(arr[2])
	if errPort != nil {
		return fmt.Errorf("Enable to create configMap. Controller port is invalid. %v", errPort)
	}
	bag.ControllerUrl = controllerUrl
	bag.ControllerPort = uint16(port)
	bag.SSLEnabled = strings.Contains(protocol, "s")
	bag.DeploysToDashboard = clusterAgent.Spec.DashboardTiers

	data, errJson := json.Marshal(bag)
	if errJson != nil {
		return fmt.Errorf("Enable to create configMap. Cannot serialize the config Bag. %v", errJson)
	}
	cm.Data = make(map[string]string)
	cm.Data["cluster-agent-config.json"] = string(data)
	e := r.client.Create(context.TODO(), cm)
	fmt.Printf("Configmap created. Error = %v\n", e)
	if e != nil {
		return fmt.Errorf("Failed to save configMap cluster-agent-config. %v", e)
	}
	return nil
}

func (r *ReconcileClusterAgent) newAgentDeployment(clusterAgent *appdynamicsv1alpha1.ClusterAgent) *appsv1.Deployment {
	fmt.Printf("BUilding deployment spec for image %s\n", clusterAgent.Spec.Image)
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
					ServiceAccountName: "appdynamics-operator",
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
							Protocol:      corev1.ProtocolTCP,
							Name:          "web-port",
						}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "agent-config",
							MountPath: "/opt/appdynamics/config/cluster-agent-config.json",
							SubPath:   "cluster-agent-config.json",
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

func (r *ReconcileClusterAgent) restartAgent(clusterAgent *appdynamicsv1alpha1.ClusterAgent) error {
	pod := &corev1.Pod{}
	key := client.ObjectKey{Namespace: clusterAgent.Namespace, Name: clusterAgent.Name}
	err := r.client.Get(context.TODO(), key, pod)
	if err != nil {
		return fmt.Errorf("Unable to get cluster-agent pod. %v", err)
	}
	//delete to force restart
	err = r.client.Delete(context.TODO(), pod)
	if err != nil {
		return fmt.Errorf("Unable to delete cluster-agent pod. %v", err)
	}
	return nil
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
