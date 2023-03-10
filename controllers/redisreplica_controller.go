/*
Copyright 2022.

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
	"time"

	redisv1beta1 "redis-operator/api/v1beta1"
	"redis-operator/services"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// RedisReplicaReconciler reconciles a RedisReplica object
type RedisReplicaReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=redis.mysite.cn,resources=redisreplicas,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=redis.mysite.cn,resources=redisreplicas/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=redis.mysite.cn,resources=redisreplicas/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RedisReplica object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *RedisReplicaReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.FromContext(ctx, "Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling RedisReplica")

	instance := &redisv1beta1.RedisReplica{}

	err := r.Client.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	rs := services.NewredisReplicaService(reqLogger, r.Client, instance)
	if err := rs.CreateConfigMap(); err != nil {
		reqLogger.Info("?????? configmap ??????", "error", err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}
	if err := rs.InitRedisStatefulset(); err != nil {
		reqLogger.Error(err, "????????? redisstatefulset ??????")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	// TODO: Finalizers ??????????????????????????????
	if !rs.RedisReplicaFinalizers() {
		reqLogger.Info("Finalizers")
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	}

	if ok, err := rs.CreateOrUpdateRedisStatefulset(); !ok || err != nil {
		if err != nil {
			reqLogger.Info("??????????????? redisstatefulset ??????", "error", err)
		} else {
			reqLogger.Info("??????????????? redisstatefulset ???????????????")
		}

		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	if err := rs.CreateRedisPodDisruptionBudget(); err != nil {
		reqLogger.Info("?????? PodDisruptionBudget ??????", "error", err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	if err := rs.CreateRedisServiceIfNotExit(); err != nil {
		reqLogger.Info("?????? RedisService ??????", "error", err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	// ?????????????????????, ??????????????????5??????,???????????????;
	if ok, err := rs.FixTerminatingPods(); !ok || err != nil {
		reqLogger.Info("FixTerminatingPods ??????", "error", err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	// TODO: ????????? cluster ??????, ????????????????????????, forget ???????????????

	// ??????pod??????, ????????????????????????return????????????
	if err := rs.CheckRedisNodeNum(); err != nil {
		reqLogger.Info("CheckRedisNodeNum ??????", "error", err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// ??????????????????, ??????????????????
	if err := rs.CheckAndSetRedisStatus(); err != nil {
		reqLogger.Error(err, "CheckAndSetRedisStatus ??????")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReplicaReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&redisv1beta1.RedisReplica{}).
		Complete(r)
}
