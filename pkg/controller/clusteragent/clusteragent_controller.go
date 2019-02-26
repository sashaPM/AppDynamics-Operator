package clusteragent

import (
	"context"
	"fmt"

	"encoding/json"
	"time"

	"strconv"
	"strings"

	appdynamicsv1alpha1 "github.com/sjeltuhin/appdynamics-operator/pkg/apis/appdynamics/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/labels"
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
	reqLogger.Info("Cluster agent spec exists. Checking the corresponding deployment...")
	// Check if the agent already exists in the namespace
	existingDeployment := &appsv1.Deployment{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: clusterAgent.Name, Namespace: clusterAgent.Namespace}, existingDeployment)
	if err != nil && errors.IsNotFound(err) {
		reqLogger.Info("Cluster agent deployment does not exist. Creating...")
		reqLogger.Info("Checking the secret")
		_, esecret := r.ensureSecret(clusterAgent, true)
		if esecret != nil {
			reqLogger.Error(esecret, "Failed to create new Cluster Agent due to secret", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
			return reconcile.Result{}, esecret
		}
		reqLogger.Info("Checking the config map")
		_, _, econfig := r.ensureConfigMap(clusterAgent, true)
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

	reqLogger.Info("Cluster agent deployment exists. Checking for deltas with the current state...")
	// Ensure the deployment spec matches the new spec
	// Differentiate between breaking changes and benign updates
	//check if secret has been recreated. if yes, restart pod
	restart, errsecret := r.ensureSecret(clusterAgent, false)
	if errsecret != nil {
		reqLogger.Error(errsecret, "Failed to get cluster agent config secret", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
		return reconcile.Result{}, errsecret
	}
	if restart {
		fmt.Println("Breaking changes detected in the secret. Restarting the cluster agent pod...")
		errRestart := r.restartAgent(clusterAgent)
		if errRestart != nil {
			reqLogger.Error(errRestart, "Failed to restart cluster agent", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
			return reconcile.Result{}, errRestart
		}
	}

	reqLogger.Info("Retrieving the agent config map")
	cm, bag, econfig := r.ensureConfigMap(clusterAgent, false)
	if econfig != nil {
		reqLogger.Error(econfig, "Failed to obtain cluster agent config map", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
		return reconcile.Result{}, econfig
	}
	breaking, benign := r.hasBreakingChanges(clusterAgent, bag)
	if breaking || benign {
		//update the configMap
		errMap := r.updateMap(cm, clusterAgent, false)
		if errMap != nil {
			return reconcile.Result{}, errMap
		}
	}
	if breaking {
		fmt.Println("Breaking changes detected. Restarting the cluster agent pod...")
		errRestart := r.restartAgent(clusterAgent)
		if errRestart != nil {
			reqLogger.Error(errRestart, "Failed to restart cluster agent", "Deployment.Namespace", clusterAgent.Namespace, "Deployment.Name", clusterAgent.Name)
			return reconcile.Result{}, errRestart
		}
	} else if benign {
		reqLogger.Info("Benign changes detected. Updating config map...")
	} else {
		reqLogger.Info("No changes detected...")
	}
	if breaking || benign {
		r.updateStatus(clusterAgent)
	}

	reqLogger.Info("Exiting reconciliation loop.")
	return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
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

func (r *ReconcileClusterAgent) ensureSecret(clusterAgent *appdynamicsv1alpha1.ClusterAgent, create bool) (bool, error) {
	secret := &corev1.Secret{}
	requiredUpdate := false
	key := client.ObjectKey{Namespace: clusterAgent.Namespace, Name: "cluster-agent-secret"}
	err := r.client.Get(context.TODO(), key, secret)
	if err != nil {
		return requiredUpdate, fmt.Errorf("Unable to get secret for cluster-agent. %v", err)
	}
	if create {
		//annotate
		err = r.annotateSecret(secret, clusterAgent)
		if err != nil {
			return requiredUpdate, err
		}
	} else {
		//check if annotation exists.
		//if it is not, update and return false to trigger restart
		if secret.Annotations["appd-cluster-agent"] != "true" {
			requiredUpdate = true
			err = r.annotateSecret(secret, clusterAgent)
			if err != nil {
				return requiredUpdate, err
			}
		}

	}
	return requiredUpdate, nil
}

func (r *ReconcileClusterAgent) annotateSecret(secret *corev1.Secret, clusterAgent *appdynamicsv1alpha1.ClusterAgent) error {
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	secret.Annotations["appd-cluster-agent"] = "true"
	err := r.client.Update(context.TODO(), secret)
	if err != nil {
		return fmt.Errorf("Unable to annotate secret for cluster-agent. %v", err)
	}
	return nil
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

func (r *ReconcileClusterAgent) ensureConfigMap(clusterAgent *appdynamicsv1alpha1.ClusterAgent, create bool) (*corev1.ConfigMap, *AppDBag, error) {
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
		errMap := r.updateMap(cm, clusterAgent, create)
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

func (r *ReconcileClusterAgent) updateMap(cm *corev1.ConfigMap, clusterAgent *appdynamicsv1alpha1.ClusterAgent, create bool) error {
	bag := getDefaultProperties()
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
	bag.DeploysToDashboard = make([]string, len(clusterAgent.Spec.DashboardTiers))
	copy(bag.DeploysToDashboard, clusterAgent.Spec.DashboardTiers)

	data, errJson := json.Marshal(bag)
	if errJson != nil {
		return fmt.Errorf("Enable to create configMap. Cannot serialize the config Bag. %v", errJson)
	}
	cm.Data = make(map[string]string)
	cm.Data["cluster-agent-config.json"] = string(data)
	var e error
	if create {
		e = r.client.Create(context.TODO(), cm)
		fmt.Printf("Configmap created. Error = %v\n", e)
	} else {
		e = r.client.Update(context.TODO(), cm)
		fmt.Printf("Configmap updated. Error = %v\n", e)
	}

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
							MountPath: "/opt/appdynamics/config/",
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
	podList := &corev1.PodList{}
	labelSelector := labels.SelectorFromSet(labelsForClusterAgent(clusterAgent))
	listOps := &client.ListOptions{
		Namespace:     clusterAgent.Namespace,
		LabelSelector: labelSelector,
	}
	err := r.client.List(context.TODO(), listOps, podList)
	if err != nil || len(podList.Items) < 1 {
		return fmt.Errorf("Unable to retrieve cluster-agent pod. %v", err)
	}
	pod := podList.Items[0]
	//delete to force restart
	err = r.client.Delete(context.TODO(), &pod)
	if err != nil {
		return fmt.Errorf("Unable to delete cluster-agent pod. %v", err)
	}
	return nil
}

func labelsForClusterAgent(clusterAgent *appdynamicsv1alpha1.ClusterAgent) map[string]string {
	return map[string]string{"app": "clusterAgent", "clusterAgent_cr": clusterAgent.Name}
}
