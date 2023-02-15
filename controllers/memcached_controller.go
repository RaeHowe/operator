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
	"fmt"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	cLog "log"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/example/memcached-operator/api/v1alpha1"
)

const memcachedFinalizer = "cache.example.com/finalizer"

// Definitions to manage status conditions
const (
	// typeAvailableMemcached represents the status of the Deployment reconciliation
	typeAvailableMemcached = "Available"
	// typeDegradedMemcached represents the status used when the custom resource is deleted and the finalizer operations are must to occur.
	typeDegradedMemcached = "Degraded"
)

// MemcachedReconciler reconciles a Memcached object
type MemcachedReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.example.com,resources=memcacheds/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Memcached object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	//实例化一个自己定义的资源对象：memcached
	memcached := &cachev1alpha1.Memcached{}

	cLog.Print(" memcached的ns信息:")
	cLog.Println(memcached.GetNamespace())

	//获取到ns下面的资源。主要是看ns下面指定的deploy是否存在
	err := r.Get(ctx, req.NamespacedName, memcached)
	if err != nil {
		if apierrors.IsNotFound(err) {
			//获取不到cr的情况
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			cLog.Println("memcached resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		cLog.Fatalln("Failed to get memcached")
		return ctrl.Result{}, err
	}

	//如果cr的status信息不是期望的话
	if memcached.Status.Conditions == nil || len(memcached.Status.Conditions) == 0 {
		meta.SetStatusCondition(&memcached.Status.Conditions, metav1.Condition{Type: typeAvailableMemcached, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, memcached); err != nil {
			cLog.Fatalln("Failed to update Memcached status")
			return ctrl.Result{}, err
		}

		//重新处理cr
		if err = r.Get(ctx, req.NamespacedName, memcached); err != nil {
			cLog.Fatalln("Failed to re-fetch memcached")
			return ctrl.Result{}, err
		}
	}

	//定义一个finalizer，当我们在删除cr资源之前，可以在这里定义一些操作。
	//finalizer概念：包含了一些条件，k8s控制器会在满足这些条件之后再把资源进行回收
	if !controllerutil.ContainsFinalizer(memcached, memcachedFinalizer) {
		cLog.Println("Adding finalizer for memcached")
		//todo: 先不写这里的逻辑
	}

	//检查Memcached这个cr资源是否被标记为要删除，判断的依据是由所设置cr的删除时间戳，如果有这个时间戳，就代表该cr要被删除了
	isMemcachedMarkedToBeDeleted := memcached.GetDeletionTimestamp() != nil
	if isMemcachedMarkedToBeDeleted {
		if controllerutil.ContainsFinalizer(memcached, memcachedFinalizer) {
			//在删除一个资源之前先进行finalizer的操作
			cLog.Println("Performing Finalizer Operations for Memcached before delete CR")

			meta.SetStatusCondition(&memcached.Status.Conditions, metav1.Condition{
				Type:    typeDegradedMemcached,
				Status:  metav1.ConditionUnknown,
				Reason:  "Finalizing",
				Message: fmt.Sprintf("Performing finalizer operations for the custom resource: %s ", memcached.Name),
			})

			if err = r.Status().Update(ctx, memcached); err != nil {
				cLog.Println(err.Error())
				cLog.Println("Failed to update memcached status")
				return ctrl.Result{}, err
			}

			r.doFinalizerOperationsForMemcached(memcached)

			//每次对cr进行操作之后，要记得重新获取cr的最新状态，不然会重新触发Reconcile的过程
			if err = r.Get(ctx, req.NamespacedName, memcached); err != nil {
				cLog.Println(err.Error())
				cLog.Println("Failed to re-fetch memcached")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&memcached.Status.Conditions, metav1.Condition{
				Type:    typeDegradedMemcached,
				Status:  metav1.ConditionTrue,
				Reason:  "Finalizing",
				Message: fmt.Sprintf("Finalizer operations for custom resource %s name were successfully accomplished", memcached.Name),
			})

			//cr状态更新
			if err = r.Status().Update(ctx, memcached); err != nil {
				cLog.Println(err.Error())
				cLog.Println("Failed to update Memcached status")
				return ctrl.Result{}, err
			}

			//成功执行操作后，删除Memcached的Finalizer
			cLog.Println("Removing Finalizer for Memcached after successfully perform the operations")
			//memcachedFinalizer是finalizer的名字
			if ok := controllerutil.RemoveFinalizer(memcached, memcachedFinalizer); !ok {
				cLog.Println(err.Error())
				cLog.Println("Failed to remove finalizer for Memcached")
				return ctrl.Result{Requeue: true}, nil
			}

			if err = r.Update(ctx, memcached); err != nil {
				cLog.Println(err.Error())
				cLog.Println("Failed to remove finalizer for Memcached")
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	//Check if the deployment already exists, if not create a new one
	found := &appsv1.Deployment{}
	//这个操作相当于就是从ns里面拿Deployment类型的资源
	err = r.Get(ctx, types.NamespacedName{
		Namespace: memcached.Namespace,
		Name:      memcached.Name,
	}, found)

	return ctrl.Result{}, nil
}

// 执行在删除cr之前的一些必要操作
func (r *MemcachedReconciler) doFinalizerOperationsForMemcached(cr *cachev1alpha1.Memcached) {
	//todo: 可以加上一些其他的资源清理操作，比如清理pvc
	r.Recorder.Event(cr, "Warning", "Deleting", fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s", cr.Name, cr.Namespace))
}

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.Memcached{}).
		Complete(r)
}