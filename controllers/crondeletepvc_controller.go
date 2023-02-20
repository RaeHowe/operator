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
	"github.com/robfig/cron"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	cLog "log"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "github.com/example/memcached-operator/api/v1alpha1"
)

// CronDeletePVCReconciler reconciles a CronDeletePVC object
type CronDeletePVCReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cache.example.com,resources=crondeletepvcs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cache.example.com,resources=crondeletepvcs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cache.example.com,resources=crondeletepvcs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CronDeletePVC object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *CronDeletePVCReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	//下面注释中，cr统一代表cachev1alpha1.CronDeletePVC
	var cronDeletePVC = cachev1alpha1.CronDeletePVC{}
	if err := r.Get(ctx, req.NamespacedName, &cronDeletePVC); err != nil {
		if apierrors.IsNotFound(err) {
			cLog.Println("can't get cron delete pvc customer resource")
			return ctrl.Result{Requeue: true}, nil
		} else {
			return ctrl.Result{}, err
		}
	}

	//1.获取到集群里面所有的pvc
	var pvcList = corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, &pvcList); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	//2.根据label标签(tidb-operator)进行过滤
	//组件label selector的map
	pvcMap := make(map[string]corev1.PersistentVolumeClaim)
	for _, pvcItem := range pvcList.Items {
		if pvcItem.Labels["app.kubernetes.io/managed-by"] == "tidb-operator" {
			pvcMap[pvcItem.Name+pvcItem.Namespace] = pvcItem
		}
	}

	//cronDeletePVC的cr里面，status.WaitingDeletePVCs集合里面包含了准备删除的pvc信息，如果在map里面包含了的话，就把集合里面的pvc元素删除掉
	for item := range cronDeletePVC.Status.WaitingDeletePVCs {
		if _, ok := pvcMap[item]; !ok {
			delete(cronDeletePVC.Status.WaitingDeletePVCs, item)
		}
	}
	//以pvcMap为准？

	//获取到所有的pod
	var pods = corev1.PodList{}
	if err := r.List(ctx, &pods); err != nil {
		return ctrl.Result{Requeue: true}, err
	}

	//pvcMap里面,不需要删除的pvc需要从map里面移除掉
	for _, pod := range pods.Items {
		for _, vo := range pod.Spec.Volumes {
			if vo.PersistentVolumeClaim != nil {
				//如果这个pod下面的pvc不是空的，就代表这个pvc不要被删除掉
				delete(pvcMap, vo.PersistentVolumeClaim.ClaimName+pod.Namespace)
				delete(cronDeletePVC.Status.WaitingDeletePVCs, vo.PersistentVolumeClaim.ClaimName+pod.Namespace)
			}
		}
	}

	//接下来pvcMap里面的就都是需要删除的pvc了

	//把pvcMap里面的pvc对象放置到cr的Status.WaitingDeletePVCs里面
	//补充WaitingDeletePVCs里面pvc的unuse时间，用于计时，满足多长时间并且未被使用的pvc，就会被删除掉
	for k, v := range pvcMap {
		l.Info("pvc" + k + "not use") //k是pvc的名字，v是具体的pvc资源
		p, ok := cronDeletePVC.Status.WaitingDeletePVCs[k]
		if !ok {
			//拿map对象失败的话
			pvc := cachev1alpha1.PVC{}
			pvc.Name = v.Name
			pvc.Namespace = v.Namespace
			pvc.UID = v.UID
			pvc.UnUseTime = &metav1.Time{Time: time.Now()}
			cronDeletePVC.Status.WaitingDeletePVCs[k] = pvc
		} else {
			if v.UID != p.UID {
				delete(cronDeletePVC.Status.WaitingDeletePVCs, k)
				pvc := cachev1alpha1.PVC{}
				pvc.Name = v.Name
				pvc.Namespace = v.Namespace
				pvc.UID = v.UID
				pvc.UnUseTime = &metav1.Time{Time: time.Now()}
				cronDeletePVC.Status.WaitingDeletePVCs[k] = pvc
			}
		}
	}

	//遍历查看cr的Status.WaitingDeletePVCs里面的pvc元素，是否到了删除时间，如果到了删除时间的话，就删掉该pvc
	for k, v := range cronDeletePVC.Status.WaitingDeletePVCs {
		unusedTime := v.UnUseTime.Time //一个pvc元素的未使用开始时间
		reserveMinutes := 0
		if cronDeletePVC.Spec.ReserveMinutes != nil {
			reserveMinutes = int(*cronDeletePVC.Spec.ReserveMinutes) //pvc剩余删除时间（从一个小时开始计时）
		}

		if unusedTime.Before(time.Now().Add(time.Duration(-1*reserveMinutes) * time.Minute)) {
			//如果超过一个小时了的话
			obj, ok := pvcMap[k]
			if ok && obj.UID == v.UID {
				l.Info("deleting pvc: Name: " + obj.Name + " namespace: " + obj.Namespace + " uid:" + string(obj.UID))

				if err := r.Delete(ctx, &obj, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
					l.Info(err.Error())
					l.Info("Delete pvc has error!!!")
				} else {
					l.Info("CronDeletePVC["+cronDeletePVC.Namespace+"/"+cronDeletePVC.Name+"] deleted pvc, ", "pvc", obj)
				}
			}

			//设置删除pvc的时间戳
			v.DeletedTime = &metav1.Time{Time: time.Now()}

			//初始化DeletedPVCs切片
			if cronDeletePVC.Status.DeletedPVCs == nil {
				cronDeletePVC.Status.DeletedPVCs = make([]cachev1alpha1.PVC, 0)
			}

			//把刚才删除的pvc对象放置到切片里面
			cronDeletePVC.Status.DeletedPVCs = append(cronDeletePVC.Status.DeletedPVCs, v)

			//从WaitingDeletePVCs（等待删除的pvc）切片里面删除掉刚刚删除的pvc对象
			delete(cronDeletePVC.Status.WaitingDeletePVCs, k)
		}
	}

	historyLimit := 0
	if cronDeletePVC.Spec.DeletedPVCHistoryLimit != nil {
		historyLimit = int(*cronDeletePVC.Spec.DeletedPVCHistoryLimit)
	}

	delNum := len(cronDeletePVC.Status.DeletedPVCs) - historyLimit
	if delNum > 0 {
		sort.Slice(cronDeletePVC.Status.DeletedPVCs, func(i, j int) bool {
			if cronDeletePVC.Status.DeletedPVCs[i].DeletedTime == nil {
				return cronDeletePVC.Status.DeletedPVCs[j].DeletedTime != nil
			}

			return cronDeletePVC.Status.DeletedPVCs[i].DeletedTime.Before(cronDeletePVC.Status.DeletedPVCs[j].DeletedTime)
		})

		cronDeletePVC.Status.DeletedPVCs = cronDeletePVC.Status.DeletedPVCs[delNum:]
	}

	cronDeletePVC.Status.LastScheduleTime = &metav1.Time{Time: time.Now()}

	getNextSchedule := func(cronDeletePvc *cachev1alpha1.CronDeletePVC, now time.Time) (next time.Time, err error) {
		//sched, err := cron.Par (cronDeletePvc.Spec.Schedule)
		sched, err := cron.ParseStandard(cronDeletePvc.Spec.Schedule)
		if err != nil {
			return time.Time{}, fmt.Errorf("unparseable schedule %q: %v", cronDeletePVC.Spec.Schedule, err)
		}

	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CronDeletePVCReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.CronDeletePVC{}).
		Complete(r)
}
