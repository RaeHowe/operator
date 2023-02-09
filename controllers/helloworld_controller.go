/*
Copyright 2023.

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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	testv1alpha1 "github.com/RaeHowe/operator/api/v1alpha1"
)

// HelloWorldReconciler reconciles a HelloWorld object
type HelloWorldReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=test.raehowe.com,resources=helloworlds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=test.raehowe.com,resources=helloworlds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=test.raehowe.com,resources=helloworlds/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HelloWorld object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *HelloWorldReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	//我的逻辑
	hellWorld := &testv1alpha1.HelloWorld{}

	//相当于kubectl get ns | grep NamespacedName
	err := r.Client.Get(ctx, req.NamespacedName, hellWorld)
	if err != nil {
		if errors.IsNotFound(err) {
			//如果找不到这个ns，说明cr被删除了
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	//确认cr下的deploy是否存在
	nginxDeployFound := &appsv1.Deployment{}

	//获取当前ns下面的deploy资源
	errDeploy := r.Client.Get(ctx, types.NamespacedName{
		Namespace: hellWorld.Namespace,
		Name:      hellWorld.Namespace,
	}, nginxDeployFound)

	if errDeploy != nil {
		//不存在的话，就需要创建deployment
		if errors.IsNotFound(errDeploy) {
			nginxDeploy := &appsv1.Deployment{
				//元数据信息
				ObjectMeta: metav1.ObjectMeta{
					Name:      hellWorld.Name,
					Namespace: hellWorld.Namespace,
				},

				//cr的spec个性信息
				Spec: appsv1.DeploymentSpec{
					Replicas: &hellWorld.Spec.Size,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"hello_name": hellWorld.Name,
						},
					},

					//deployment里面的template，deploy控制的pod的数量，如果不满足replica数量的话，就会根据这个template去创建一个pod出来
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"hello_name": hellWorld.Name,
							},
						},

						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Image: "nginx",
									Name:  "nginx",
									Ports: []corev1.ContainerPort{
										{
											ContainerPort: 80,
											Name:          "nginx",
										},
									},
								},
							},
						},
					},
				},
			}

			controllerutil.SetControllerReference(hellWorld, nginxDeploy, r.Scheme)
			if err1 := r.Client.Create(ctx, nginxDeploy); err1 != nil {
				return ctrl.Result{}, err1
			}

			return ctrl.Result{Requeue: true}, nil
		} else {
			return ctrl.Result{}, errDeploy
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HelloWorldReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&testv1alpha1.HelloWorld{}).
		Complete(r)
}
