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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	cLog "log"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/example/memcached-operator/api/v1alpha1"
)

/*
	k8s.io/api/apps/v1包里面包含了deployment, stateful set, replicaset
	k8s.io/api/core/v1包里面包含了除deployment, stateful set, replicaset之外大部分的系统资源，例如pod,node,cm,
	k8s.io/apimachinery/pkg/api/meta包的主要功能是设置一个资源的status.conditions的状态值
*/

const memcachedFinalizer = "cache.example.com/finalizer"
const MemcachedImage = "memcached:1.4.36-alpine"

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

	//获取到ns下面的资源，判断ns下面memcached这个cr资源是否存在
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
		//SetStatusCondition方法主要是设置pod,deployment等资源里面status.Condition里面的状态值
		//可以了解一下Condition
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
	//这个操作相当于就是从ns里面拿Deployment类型的资源，拿到的资源给到deploy这个变量
	err = r.Get(ctx, types.NamespacedName{
		Namespace: memcached.Namespace,
		Name:      memcached.Name,
	}, found)

	if err != nil && apierrors.IsNotFound(err) {
		dep, err := r.deploymentForMemcached(memcached)
		if err != nil {
			//部署deploy失败
			cLog.Println(err.Error())
			cLog.Println("Failed to define new Deployment resource for Memcached")

			//部署deploy失败的话，就把memcached的conditions状态修改一下
			meta.SetStatusCondition(&memcached.Status.Conditions, metav1.Condition{
				Type:    typeAvailableMemcached,
				Status:  metav1.ConditionFalse,
				Reason:  "Reconciling",
				Message: fmt.Sprintf("Failed to create Deployment for the custom resource (%s): (%s)", memcached.Name, err),
			})

			//修改协调器状态
			if err = r.Status().Update(ctx, memcached); err != nil {
				cLog.Println(err.Error())
				cLog.Println("Failed to update Memcached status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		//部署deploy成功
		cLog.Println("Creating a new Deployment",
			"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
		//在k8s集群里面创建一个资源出来
		if err = r.Create(ctx, dep); err != nil {
			cLog.Println(err.Error())
			cLog.Println("Failed to create new Deployment",
				"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		//从集群拿deploy资源获取失败，但并不是deploy不存在的原因
		cLog.Println(err.Error())
		cLog.Println("Failed to get Deployment")
		return ctrl.Result{}, err
	}

	//下面判断部署出来的memcached数量是否cr里面要求的数量保持一致
	size := memcached.Spec.Size //cr里面要求的size，以这个为准
	if *found.Spec.Replicas != size {
		//如果不一致的话，把deployment的size设置为cr要求的数量。然后直接通过协调器去更新
		found.Spec.Replicas = &size
		if err = r.Update(ctx, found); err != nil {
			cLog.Println(err.Error())
			cLog.Println("Failed to update Deployment",
				"Deployment.Namespace", found.Namespace, "Deployment.Name", found.Name)

			//更新失败的话，就修改memcached的状态为失败状态
			if err := r.Get(ctx, req.NamespacedName, memcached); err != nil {
				return ctrl.Result{}, err
			}

			//修改状态
			meta.SetStatusCondition(&memcached.Status.Conditions, metav1.Condition{
				Type:    typeAvailableMemcached,
				Status:  metav1.ConditionFalse,
				Reason:  "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", memcached.Name, err),
			})

			if err := r.Status().Update(ctx, memcached); err != nil {
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		return ctrl.Result{Requeue: true}, nil
	}

	//如果都成功了，就设置一个success状态
	meta.SetStatusCondition(&memcached.Status.Conditions, metav1.Condition{
		Type:    typeAvailableMemcached,
		Status:  metav1.ConditionTrue,
		Reason:  "Reconciling",
		Message: fmt.Sprintf("Deployment for custom resource (%s) with %d replicas created successfully", memcached.Name, size),
	})

	if err := r.Status().Update(ctx, memcached); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MemcachedReconciler) deploymentForMemcached(memcached *cachev1alpha1.Memcached) (*appsv1.Deployment, error) {
	ls := labelsForMemcached(memcached.Name)
	replicas := memcached.Spec.Size //获取到memcached的副本数（从memcached的cr.yaml里面拿到的）

	deploy := &appsv1.Deployment{
		//deploy的元数据信息
		ObjectMeta: metav1.ObjectMeta{
			Name:      memcached.Name,
			Namespace: memcached.Namespace,
		},

		//deploy的spec信息
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas, //直接从cr.yaml里面拿到的cr副本数放置到deploy里面来
			Selector: &metav1.LabelSelector{
				/**
				spec:
				  selector:
					matchLabels:
					  app.kubernetes.io/component: discovery
					  app.kubernetes.io/instance: tidb-6akptd44c4
					  app.kubernetes.io/managed-by: tidb-operator
					  app.kubernetes.io/name: tidb-cluster
				*/
				MatchLabels: ls,
			},

			//deploy的yaml文件里面spec.template对应的内容
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: ls,
				}, //container的name
				Spec: corev1.PodSpec{
					//pod亲和性方面配置
					Affinity: &corev1.Affinity{
						NodeAffinity: &corev1.NodeAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
								NodeSelectorTerms: []corev1.NodeSelectorTerm{
									{
										MatchExpressions: []corev1.NodeSelectorRequirement{
											{
												Key:      "kubernetes.io/arch",
												Operator: "In",
												Values:   []string{"amd64", "arm64", "ppc64le", "s390x"},
											},
											{
												Key:      "kubernetes.io/os",
												Operator: "In",
												Values:   []string{"linux"},
											},
										},
									},
								},
							},
						},
						PodAffinity:     nil,
						PodAntiAffinity: nil,
					},
					//定义pod和container的权限和访问控制
					/*
						SecurityContext 可以应用于 Container 和 Pod 维度：
							在 Pod 上设置的安全性配置会应用到 Pod 中所有 Container 上，并且会还会影响 Volume
							在 Container 上设置的安全性配置仅适用于该容器本身，不会影响到其他容器以及 Pod 的 Volume
					*/
					SecurityContext: &corev1.PodSecurityContext{},
					Containers: []corev1.Container{
						{
							Image:           MemcachedImage,
							Name:            "memcached",
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: memcached.Spec.ContainerPort,
									Name:          "memcached",
								},
							},
							Command: []string{"memcached", "-m=64", "-o", "modern", "-v"},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(memcached, deploy, r.Scheme); err != nil {
		return nil, err
	}

	return deploy, nil
}

func labelsForMemcached(name string) map[string]string {
	var imageTag = strings.Split(MemcachedImage, ":")[1] //1.4.36-alpine

	return map[string]string{"app.kubernetes.io/name": "Memcached",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/version":    imageTag,
		"app.kubernetes.io/part-of":    "memcached-operator",
		"app.kubernetes.io/created-by": "controller-manager"}
}

// 执行在删除cr之前的一些必要操作
func (r *MemcachedReconciler) doFinalizerOperationsForMemcached(cr *cachev1alpha1.Memcached) {
	//todo: 可以加上一些其他的资源清理操作，比如清理pvc
	r.Recorder.Event(cr, "Warning", "Deleting", fmt.Sprintf("Custom Resource %s is being deleted from the namespace %s", cr.Name, cr.Namespace))
}

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// SetupWithManager里面定义的内容，是你的operator涉及到了对哪些资源进行操作的资源集合
	return ctrl.NewControllerManagedBy(mgr).
		//NewControllerManagedBy
		For(&cachev1alpha1.Memcached{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
