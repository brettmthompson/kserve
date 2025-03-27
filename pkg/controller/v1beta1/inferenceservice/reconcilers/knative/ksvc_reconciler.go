/*
Copyright 2021 The KServe Authors.

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

package knative

import (
	"context"
	"fmt"
	"strconv"

	"github.com/kserve/kserve/pkg/apis/serving/v1beta1"
	"github.com/kserve/kserve/pkg/constants"
	"github.com/kserve/kserve/pkg/utils"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"knative.dev/pkg/kmp"
	"knative.dev/serving/pkg/apis/autoscaling"
	knserving "knative.dev/serving/pkg/apis/serving"
	knservingv1 "knative.dev/serving/pkg/apis/serving/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var log = logf.Log.WithName("KsvcReconciler")

var managedKsvcAnnotations = map[string]bool{
	constants.RollOutDurationAnnotationKey: true,
	// Required for the integration of Openshift Serverless with Openshift Service Mesh
	constants.KnativeOpenshiftEnablePassthroughKey: true,
}

type KsvcReconciler struct {
	client          client.Client
	scheme          *runtime.Scheme
	Service         *knservingv1.Service
	componentExt    *v1beta1.ComponentExtensionSpec
	componentStatus v1beta1.ComponentStatusSpec
}

func NewKsvcReconciler(client client.Client,
	scheme *runtime.Scheme,
	componentMeta metav1.ObjectMeta,
	componentExt *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec,
	componentStatus v1beta1.ComponentStatusSpec,
	disallowedLabelList []string) (*KsvcReconciler, error) {
	ksvc, err := createKnativeService(client, componentMeta, componentExt, podSpec, componentStatus, disallowedLabelList)
	if err != nil {
		log.Error(err, "failed to create knative service", "inference service", componentMeta.Name)
		return nil, err
	}
	return &KsvcReconciler{
		client:          client,
		scheme:          scheme,
		Service:         ksvc,
		componentExt:    componentExt,
		componentStatus: componentStatus,
	}, nil
}

func createKnativeService(client client.Client,
	componentMeta metav1.ObjectMeta,
	componentExtension *v1beta1.ComponentExtensionSpec,
	podSpec *corev1.PodSpec,
	componentStatus v1beta1.ComponentStatusSpec,
	disallowedLabelList []string) (*knservingv1.Service, error) {
	annotations := componentMeta.GetAnnotations()

	err := setAutoScalingAnnotations(client, annotations, componentExtension)
	if err != nil {
		return nil, err
	}

	// ksvc metadata.annotations
	// rollout-duration must be put under metadata.annotations
	ksvcAnnotations := make(map[string]string)
	for ksvcAnnotationKey := range managedKsvcAnnotations {
		if value, ok := annotations[ksvcAnnotationKey]; ok {
			ksvcAnnotations[ksvcAnnotationKey] = value
			delete(annotations, ksvcAnnotationKey)
		}
	}

	lastRolledoutRevision := componentStatus.LatestRolledoutRevision

	// Log component status and canary traffic percent
	log.Info("revision status:", "LatestRolledoutRevision", componentStatus.LatestRolledoutRevision,
		"LatestReadyRevision", componentStatus.LatestReadyRevision,
		"LatestCreatedRevision", componentStatus.LatestCreatedRevision,
		"PreviousRolledoutRevision", componentStatus.PreviousRolledoutRevision,
		"CanaryTrafficPercent", componentExtension.CanaryTrafficPercent)

	trafficTargets := []knservingv1.TrafficTarget{}
	// Split traffic when canary traffic percent is specified
	if componentExtension.CanaryTrafficPercent != nil && lastRolledoutRevision != "" {
		latestTarget := knservingv1.TrafficTarget{
			LatestRevision: proto.Bool(true),
			Percent:        proto.Int64(*componentExtension.CanaryTrafficPercent),
		}
		if value, ok := annotations[constants.EnableRoutingTagAnnotationKey]; ok && value == "true" {
			latestTarget.Tag = "latest"
		}
		trafficTargets = append(trafficTargets, latestTarget)

		if *componentExtension.CanaryTrafficPercent < 100 {
			remainingTraffic := 100 - *componentExtension.CanaryTrafficPercent
			canaryTarget := knservingv1.TrafficTarget{
				RevisionName:   lastRolledoutRevision,
				LatestRevision: proto.Bool(false),
				Percent:        proto.Int64(remainingTraffic),
				Tag:            "prev",
			}
			trafficTargets = append(trafficTargets, canaryTarget)
		}
	} else {
		// blue-green rollout
		latestTarget := knservingv1.TrafficTarget{
			LatestRevision: proto.Bool(true),
			Percent:        proto.Int64(100),
		}
		if value, ok := annotations[constants.EnableRoutingTagAnnotationKey]; ok && value == "true" {
			latestTarget.Tag = "latest"
		}
		trafficTargets = append(trafficTargets, latestTarget)
	}

	labels := utils.Filter(componentMeta.Labels, func(key string) bool {
		return !utils.Includes(disallowedLabelList, key)
	})

	service := &knservingv1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        componentMeta.Name,
			Namespace:   componentMeta.Namespace,
			Labels:      componentMeta.Labels,
			Annotations: ksvcAnnotations,
		},
		Spec: knservingv1.ServiceSpec{
			ConfigurationSpec: knservingv1.ConfigurationSpec{
				Template: knservingv1.RevisionTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels:      labels,
						Annotations: annotations,
					},
					Spec: knservingv1.RevisionSpec{
						// If timeoutSeconds is not set by isvc(componentExtension.TimeoutSeconds is nil), Knative
						// Serving will set timeoutSeconds to the default value.
						TimeoutSeconds: componentExtension.TimeoutSeconds,
						// If timeoutSeconds is set by isvc, set ResponseStartTimeoutSeconds to the same value.
						// If timeoutSeconds is not set by isvc, set ResponseStartTimeoutSeconds to empty.
						ResponseStartTimeoutSeconds: componentExtension.TimeoutSeconds,
						ContainerConcurrency:        componentExtension.ContainerConcurrency,
						PodSpec:                     *podSpec,
					},
				},
			},
			RouteSpec: knservingv1.RouteSpec{
				Traffic: trafficTargets,
			},
		},
	}
	return service, nil
}

func reconcileKsvc(desired *knservingv1.Service, existing *knservingv1.Service) error {
	// Return if no differences to reconcile.
	if semanticEquals(desired, existing) {
		return nil
	}

	// Reconcile differences and update
	// knative mutator defaults the enableServiceLinks to false which would generate a diff despite no changes on desired knative service
	// https://github.com/knative/serving/blob/main/pkg/apis/serving/v1/revision_defaults.go#L134
	if desired.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks == nil &&
		existing.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks != nil &&
		!*existing.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks {
		desired.Spec.ConfigurationSpec.Template.Spec.EnableServiceLinks = proto.Bool(false)
	}
	diff, err := kmp.SafeDiff(desired.Spec.ConfigurationSpec, existing.Spec.ConfigurationSpec)
	if err != nil {
		return errors.Wrapf(err, "failed to diff knative service configuration spec")
	}
	log.Info("knative service configuration diff (-desired, +observed):", "diff", diff)
	existing.Spec.ConfigurationSpec = desired.Spec.ConfigurationSpec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.Spec.Traffic = desired.Spec.Traffic
	for ksvcAnnotationKey := range managedKsvcAnnotations {
		if desiredValue, ok := desired.ObjectMeta.Annotations[ksvcAnnotationKey]; ok {
			existing.ObjectMeta.Annotations[ksvcAnnotationKey] = desiredValue
		} else {
			delete(existing.ObjectMeta.Annotations, ksvcAnnotationKey)
		}
	}
	return nil
}

func (r *KsvcReconciler) Reconcile() (*knservingv1.ServiceStatus, error) {
	desired := r.Service
	existing := &knservingv1.Service{}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		log.Info("Updating knative service", "namespace", desired.Namespace, "name", desired.Name)
		if err := r.client.Get(context.TODO(), types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing); err != nil {
			return err
		}

		// Set ResourceVersion which is required for update operation.
		desired.ResourceVersion = existing.ResourceVersion
		// Add immutable annotations to avoid validation error during dry-run update.
		desired.Annotations[knserving.CreatorAnnotation] = existing.Annotations[knserving.CreatorAnnotation]
		desired.Annotations[knserving.UpdaterAnnotation] = existing.Annotations[knserving.UpdaterAnnotation]

		// Do a dry-run update to avoid diffs generated by default values introduced by knative's defaulter webhook.
		// This will populate our local knative service object with any default values
		// that are present on the remote version.
		if err := r.client.Update(context.TODO(), desired, client.DryRunAll); err != nil {
			// log only if it is not resource conflict error to avoid spamming
			if !apierr.IsConflict(err) {
				log.Error(err, "Failed to perform dry-run update of knative service", "service", desired.Name)
			}
			return err
		}
		if err := reconcileKsvc(desired, existing); err != nil {
			return err
		}
		return r.client.Update(context.TODO(), existing)
	})
	if err != nil {
		// Create service if it does not exist
		if apierr.IsNotFound(err) {
			log.Info("Creating knative service", "namespace", desired.Namespace, "name", desired.Name)
			return &desired.Status, r.client.Create(context.TODO(), desired)
		}
		return &existing.Status, errors.Wrapf(err, "fails to reconcile knative service")
	}
	return &existing.Status, nil
}

func semanticEquals(desiredService, service *knservingv1.Service) bool {
	for ksvcAnnotationKey := range managedKsvcAnnotations {
		existingValue, ok1 := service.ObjectMeta.Annotations[ksvcAnnotationKey]
		desiredValue, ok2 := desiredService.ObjectMeta.Annotations[ksvcAnnotationKey]
		if ok1 != ok2 || existingValue != desiredValue {
			return false
		}
	}
	return equality.Semantic.DeepEqual(desiredService.Spec.ConfigurationSpec, service.Spec.ConfigurationSpec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredService.Spec.RouteSpec, service.Spec.RouteSpec)
}

// setAutoScalingAnnotations checks the knative autoscaler configuration defined in the config-autoscaler
// configmap in the knative-serving namespace and compares the values to the autoscaling configuration requested
// for the ISVC. It then sets the necessary annotations for the desired autoscaling configuration.
func setAutoScalingAnnotations(client client.Client,
	annotations map[string]string,
	componentExtension *v1beta1.ComponentExtensionSpec) error {

	// If a minReplicas value is not set for the ISVC, then use the default min-scale value of 1.
	var revisionMinScale int
	if componentExtension.MinReplicas == nil {
		annotations[constants.MinScaleAnnotationKey] = fmt.Sprint(constants.DefaultMinReplicas)
		revisionMinScale = constants.DefaultMinReplicas
	} else {
		annotations[constants.MinScaleAnnotationKey] = fmt.Sprint(*componentExtension.MinReplicas)
		revisionMinScale = *componentExtension.MinReplicas
	}

	annotations[constants.MaxScaleAnnotationKey] = fmt.Sprint(componentExtension.MaxReplicas)

	// User can pass down scaling class annotation to overwrite the default scaling KPA
	if _, ok := annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}

	if componentExtension.ScaleTarget != nil {
		annotations[autoscaling.TargetAnnotationKey] = fmt.Sprint(*componentExtension.ScaleTarget)
	}

	if componentExtension.ScaleMetric != nil {
		annotations[autoscaling.MetricAnnotationKey] = fmt.Sprint(*componentExtension.ScaleMetric)
	}

	// Retrive the allow-zero-initial-scale and initial-scale values from the config-autoscaler configmap.
	// If their respective keys, are not found in the configmap, use the knative default values.
	allowZeroInitialScale := "false"
	globalInitialScale := "1"
	autoscalerConfig := &corev1.ConfigMap{}
	err := client.Get(context.TODO(), types.NamespacedName{Name: "config-autoscaler", Namespace: "knative-serving"}, autoscalerConfig)
	if err != nil {
		log.Error(err, "failed to retrieve config-autoscaler configmap from the knative-serving namespace during knative service creation.")
		return err
	}
	if autoscalerConfig.Data != nil {
		if configuredAllowZeroInitialScale, ok := autoscalerConfig.Data["allow-zero-initial-scale"]; ok {
			allowZeroInitialScale = configuredAllowZeroInitialScale
		}
		if configuredInitialScale, ok := autoscalerConfig.Data["initial-scale"]; ok {
			globalInitialScale = configuredInitialScale
		}
	}

	initialScaleInt, err := strconv.Atoi(globalInitialScale)
	if err != nil {
		log.Error(err, "failed to convert configured knative global initial-scale value to an integer", "initial-scale", globalInitialScale)
		return err
	}

	// Provide transparency to users while aligning with knatives expected behaviour, log a warning when the knative global
	// initial-scale value exceeds the requested minScale value for the ISVC.
	if initialScaleInt > revisionMinScale {
		log.Info("WARNING: knative is globally configured with an initial-scale value that is greater than the requested min-scale for the ISVC",
			"initial-scale", initialScaleInt,
			"min-scale", revisionMinScale)
	}

	// knative will choose the larger of min-scale and initial-scale as the initial target scale for a knative Revision.
	// When min-scale is 0, if allow-zeron-initial scale is true, set initial-scale to 0 for the created knative revision.
	// This will prevent any pods from being created to initialize a knative revision when an ISVC has minReplicas set to 0.
	// Configuring scaling for knative: https://knative.dev/docs/serving/autoscaling/scale-bounds/#initial-scale
	if revisionMinScale == 0 {
		if allowZeroInitialScale == "true" {
			log.Info("kserve will override the global knative configuration for initial-scale on a per revision basis when an ISVC is requested with min-scale 0",
				"revision-intitial-scale", "0",
				"global-initial-scale", globalInitialScale)
			annotations[constants.InitialScaleAnnotationKey] = "0"
		} else {
			log.Info("WARNING: The current knative global configuration does not allow zero initial scale.",
				"allow-zero-initial-scale", allowZeroInitialScale,
				"initial-scale", globalInitialScale)
		}
	} else if componentExtension.MaxReplicas != 0 && initialScaleInt > componentExtension.MaxReplicas {
		log.Info("WARNING: knative is globally configured with an initial-scale value that is greater than the requested max-scale for the ISVC",
			"initial-scale", initialScaleInt,
			"max-scale", componentExtension.MaxReplicas)
		log.Info("setting initial-scale to the same value as max-scale for the requested ISVC",
			"initial-scale", componentExtension.MaxReplicas)
		annotations[constants.InitialScaleAnnotationKey] = fmt.Sprint(componentExtension.MaxReplicas)
	}

	return nil
}
