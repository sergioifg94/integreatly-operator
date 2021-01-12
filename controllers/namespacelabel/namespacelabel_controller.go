/*
Copyright 2020 Red Hat, Inc.

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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"

	"github.com/integr8ly/integreatly-operator/pkg/resources"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	log           = logf.Log.WithName("controller_namespace_label")
	configMapName = "cloud-resources-aws-strategies"
)

//  patchStringValue specifies a patch operation for a string.
type patchStringValue struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

type network struct {
	Production struct {
		CreateStrategy struct {
			CidrBlock string `json:"CidrBlock"`
		} `json:"createStrategy"`
	} `json:"production"`
}

// // Add creates a new namespacelabel Controller and adds it to the Manager. The Manager will set fields on the Controller
// // and Start it when the Manager is Started.
// func Add(mgr manager.Manager) error {
// 	reconcile, err := newReconciler(mgr)
// 	if err != nil {
// 		return err
// 	}
// 	return add(mgr, reconcile)
// }

var (
	// Map that associates the labels in the namespace to actions to perform
	// - Keys are labels that might be set to the namespace object
	// - Values are functions that receive the value of the label, the reconcile request,
	//   and the reconciler instance.
	namespaceLabelBasedActions = map[string]func(string, ctrl.Request, *NamespaceLabelReconciler) error{
		// Uninstall RHMI
		"api.openshift.com/addon-rhmi-operator-delete": Uninstall,
		// Uninstall MAO
		"api.openshift.com/addon-managed-api-service-delete": Uninstall,
		// Update CIDR value
		"cidr": CheckCidrValueAndUpdate,
	}
)

func (r *NamespaceLabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(r)
}

func New(mgr manager.Manager) *NamespaceLabelReconciler {
	watchNS, err := resources.GetWatchNamespace()
	if err != nil {
		panic("could not get watch namespace from namespacelabel controller")
	}
	namespaceSegments := strings.Split(watchNS, "-")
	namespacePrefix := strings.Join(namespaceSegments[0:2], "-") + "-"
	operatorNs := namespacePrefix + "operator"

	return &NamespaceLabelReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		Log:               ctrl.Log.WithName("controllers").WithName("Namespace"),
		operatorNamespace: operatorNs,
	}
}

// // newReconciler returns a new reconcile.Reconciler
// func newReconciler(mgr manager.Manager) (ctrl.Reconciler, error) {
// 	ctx, cancel := context.WithCancel(context.Background())
// 	watchNS, err := k8sutil.GetWatchNamespace()
// 	if err != nil {
// 		cancel()
// 		return nil, errors.Wrap(err, "could not get watch namespace from namespacelabel controller")
// 	}
// 	namespaceSegments := strings.Split(watchNS, "-")
// 	namespacePrefix := strings.Join(namespaceSegments[0:2], "-") + "-"
// 	operatorNs := namespacePrefix + "operator"

// 	return &ReconcileNamespaceLabel{
// 		mgr:               mgr,
// 		client:            mgr.GetClient(),
// 		scheme:            mgr.GetScheme(),
// 		operatorNamespace: operatorNs,
// 		context:           ctx,
// 		cancel:            cancel,
// 	}, nil
// }

// // add adds a new Controller to mgr with r as the reconcile.Reconciler
// func add(mgr manager.Manager, r reconcile.Reconciler) error {
// 	// Create a new controller
// 	c, err := controller.New("namespace-label-controller", mgr, controller.Options{Reconciler: r})
// 	if err != nil {
// 		return err
// 	}

// 	// Watch for changes to primary resource
// 	err = c.Watch(&source.Kind{Type: &corev1.Namespace{}}, &handler.EnqueueRequestForObject{})
// 	if err != nil {
// 		return err
// 	}

// 	return nil
// }

// ReconcileNamespaceLabel reconciles a namespace label object
type NamespaceLabelReconciler struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	k8sclient.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	operatorNamespace string
	controller        controller.Controller
}

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=list;get;watch;update

// Reconcile : The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *NamespaceLabelReconciler) Reconcile(request ctrl.Request) (ctrl.Result, error) {
	_ = context.Background()
	log := r.Log.WithValues("namespacelabel", request.NamespacedName)

	if request.NamespacedName.Name == r.operatorNamespace {
		log.Info("Reconciling namespace labels")

		ns, err := GetNS(context.TODO(), r.operatorNamespace, r.Client)
		if err != nil {
			log.Error(err, "could not retrieve %s namespace")
		}
		err = r.CheckLabel(ns, request)

		if err != nil {
			return ctrl.Result{}, err
		}

		log.Info("Reconciling namespace labels completed")
	}
	return ctrl.Result{Requeue: true, RequeueAfter: 1 * time.Minute}, nil
}

// GetNS gets the specified corev1.Namespace from the k8s API server
func GetNS(ctx context.Context, namespace string, client k8sclient.Client) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	err := client.Get(ctx, k8sclient.ObjectKey{Name: ns.Name}, ns)
	if err == nil {
		// workaround for https://github.com/kubernetes/client-go/issues/541
		ns.TypeMeta = metav1.TypeMeta{Kind: "Namespace", APIVersion: metav1.SchemeGroupVersion.Version}
	}
	return ns, err
}

// CheckLabel Checks namespace for labels and
func (r *NamespaceLabelReconciler) CheckLabel(o metav1.Object, request ctrl.Request) error {
	for k, v := range o.GetLabels() {
		action, ok := namespaceLabelBasedActions[k]
		if !ok {
			continue
		}

		if err := action(v, request, r); err != nil {
			return err
		}
	}

	return nil
}

// Uninstall deletes rhmi cr when uninstall label is set
func Uninstall(v string, request ctrl.Request, r *NamespaceLabelReconciler) error {
	if v != "true" {
		return nil
	}

	log.Info("Uninstall label has been set")

	rhmiCr, err := resources.GetRhmiCr(r.Client, context.TODO(), request.NamespacedName.Namespace)
	if err != nil {
		// Error reading the object - requeue the request.
		return err
	}
	if rhmiCr == nil {
		// Request object not found, could have been deleted after reconcile request.
		// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
		// Return and don't requeue
		return nil
	}

	if rhmiCr.DeletionTimestamp == nil {
		log.Info("Deleting RHMI CR")
		err := r.Delete(context.TODO(), rhmiCr)
		if err != nil {
			log.Error(err, "failed to delete RHMI CR")
		}
	}
	return nil
}

// CheckCidrValueAndUpdate Checks cidr value and updates it in the configmap if the config map value is ""
func CheckCidrValueAndUpdate(value string, request ctrl.Request, r *NamespaceLabelReconciler) error {
	log.Info(fmt.Sprintf("Cidr value : %v, passed in as a namespace label", value))
	cfgMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: request.NamespacedName.Name,
		},
	}

	err := r.Get(context.TODO(), k8sclient.ObjectKey{Name: configMapName, Namespace: request.NamespacedName.Name}, cfgMap)
	if err != nil {
		return err
	}
	data := []byte(cfgMap.Data["_network"])

	var cfgMapData network

	// Unmarshal or Decode the JSON to the interface.
	err = json.Unmarshal([]byte(data), &cfgMapData)
	if err != nil {
		log.Error(err, "failed to unmarshall json")
	}

	cidr := cfgMapData.Production.CreateStrategy.CidrBlock

	if cidr != "" {
		log.Info(fmt.Sprintf("Cidr value is already set to : %v , not updating", cidr))
		return nil
	}

	// replace - character from label with / so that the cidr value is set correctly.
	// / is not a valid character in namespace label values.
	newCidr := strings.Replace(value, "-", "/", -1)
	log.Info(fmt.Sprintf("No cidr has been set in configmap yet, Setting cidr from namespace label : %v", newCidr))

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{CurrentContext: ""}).ClientConfig()
	if err != nil {
		return err
	}

	// Creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	cfgMapData.Production.CreateStrategy.CidrBlock = newCidr
	dataValue, err := json.Marshal(cfgMapData)

	if err != nil {
		return err
	}

	payload := []patchStringValue{{
		Op:    "add",
		Path:  "/data/_network",
		Value: string(dataValue),
	}}

	payloadBytes, _ := json.Marshal(payload)
	_, err = clientset.
		CoreV1().
		ConfigMaps(request.NamespacedName.Name).
		Patch(context.TODO(), configMapName, types.JSONPatchType, payloadBytes, metav1.PatchOptions{})

	if err != nil {
		return err
	}
	return nil
}
