/*
Copyright 2022 The KServe Authors.

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

package inferencegraph

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	v1alpha1api "github.com/kserve/kserve/pkg/apis/serving/v1alpha1"
	"github.com/kserve/kserve/pkg/constants"
	knutils "github.com/kserve/kserve/pkg/controller/v1beta1/inferenceservice/reconcilers/knative"
	"github.com/kserve/kserve/pkg/utils"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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

var log = logf.Log.WithName("GraphKsvcReconciler")

type GraphKnativeServiceReconciler struct {
	client  client.Client
	scheme  *runtime.Scheme
	Service *knservingv1.Service
}

func NewGraphKnativeServiceReconciler(client client.Client,
	scheme *runtime.Scheme,
	ksvc *knservingv1.Service) *GraphKnativeServiceReconciler {
	return &GraphKnativeServiceReconciler{
		client:  client,
		scheme:  scheme,
		Service: ksvc,
	}
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
		return errors.Wrapf(err, "failed to diff inference graph knative service configuration spec")
	}
	log.Info("inference graph knative service configuration diff (-desired, +observed):", "diff", diff)
	existing.Spec.ConfigurationSpec = desired.Spec.ConfigurationSpec
	existing.ObjectMeta.Labels = desired.ObjectMeta.Labels
	existing.Spec.Traffic = desired.Spec.Traffic
	return nil
}

func (r *GraphKnativeServiceReconciler) Reconcile() (*knservingv1.ServiceStatus, error) {
	desired := r.Service
	existing := &knservingv1.Service{}

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		log.Info("Updating inference graph knative service", "namespace", desired.Namespace, "name", desired.Name)
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
		if apierr.IsNotFound(err) {
			log.Info("Creating inference graph knative service", "namespace", desired.Namespace, "name", desired.Name)
			return &desired.Status, r.client.Create(context.TODO(), desired)
		}
		return &existing.Status, errors.Wrapf(err, "fails to reconcile inference graph knative service")
	}
	return &existing.Status, nil
}

func semanticEquals(desiredService, service *knservingv1.Service) bool {
	return equality.Semantic.DeepEqual(desiredService.Spec.ConfigurationSpec, service.Spec.ConfigurationSpec) &&
		equality.Semantic.DeepEqual(desiredService.ObjectMeta.Labels, service.ObjectMeta.Labels) &&
		equality.Semantic.DeepEqual(desiredService.Spec.RouteSpec, service.Spec.RouteSpec)
}

func createKnativeService(client client.Client, componentMeta metav1.ObjectMeta, graph *v1alpha1api.InferenceGraph, config *RouterConfig) (*knservingv1.Service, error) {
	bytes, err := json.Marshal(graph.Spec)
	if err != nil {
		return nil, errors.Wrapf(err, "fails to marshal inference graph spec to json")
	}
	annotations := componentMeta.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	labels := componentMeta.GetLabels()
	if labels == nil {
		labels = make(map[string]string) //nolint:ineffassign, staticcheck
	}

	err = setAutoScalingAnnotations(client, annotations)
	if err != nil {
		return nil, errors.Wrapf(err, "fails to set autoscaling annotations for knative service")
	}

	// ksvc metadata.annotations
	ksvcAnnotations := make(map[string]string)

	if value, ok := annotations[constants.KnativeOpenshiftEnablePassthroughKey]; ok {
		ksvcAnnotations[constants.KnativeOpenshiftEnablePassthroughKey] = value
		delete(annotations, constants.KnativeOpenshiftEnablePassthroughKey)
	}

	labels = utils.Filter(componentMeta.Labels, func(key string) bool {
		return !utils.Includes(constants.RevisionTemplateLabelDisallowedList, key)
	})
	labels[constants.InferenceGraphLabel] = componentMeta.Name
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
						TimeoutSeconds: graph.Spec.TimeoutSeconds,
						PodSpec: v1.PodSpec{
							Containers: []v1.Container{
								{
									Image: config.Image,
									Args: []string{
										"--graph-json",
										string(bytes),
									},
									Resources: constructResourceRequirements(*graph, *config),
									SecurityContext: &v1.SecurityContext{
										Privileged:               proto.Bool(false),
										RunAsNonRoot:             proto.Bool(true),
										ReadOnlyRootFilesystem:   proto.Bool(true),
										AllowPrivilegeEscalation: proto.Bool(false),
										Capabilities: &v1.Capabilities{
											Drop: []v1.Capability{v1.Capability("ALL")},
										},
									},
									VolumeMounts: []v1.VolumeMount{
										{
											Name:      "openshift-service-ca-bundle",
											MountPath: "/etc/odh/openshift-service-ca-bundle",
										},
									},
									Env: []v1.EnvVar{
										{
											Name:  "SSL_CERT_FILE",
											Value: "/etc/odh/openshift-service-ca-bundle/service-ca.crt",
										},
									},
									ReadinessProbe: constants.GetRouterReadinessProbe(),
								},
							},
							Volumes: []v1.Volume{
								{
									Name: "openshift-service-ca-bundle",
									VolumeSource: v1.VolumeSource{
										ConfigMap: &v1.ConfigMapVolumeSource{
											LocalObjectReference: v1.LocalObjectReference{
												Name: constants.OpenShiftServiceCaConfigMapName,
											},
										},
									},
								},
							},
							Affinity:                     graph.Spec.Affinity,
							AutomountServiceAccountToken: proto.Bool(false), // Inference graph does not need access to api server
						},
					},
				},
			},
		},
	}

	// Only adding this env variable "PROPAGATE_HEADERS" if router's headers config has the key "propagate"
	value, exists := config.Headers["propagate"]
	if exists {
		propagateEnv := v1.EnvVar{
			Name:  constants.RouterHeadersPropagateEnvVar,
			Value: strings.Join(value, ","),
		}

		service.Spec.ConfigurationSpec.Template.Spec.PodSpec.Containers[0].Env = append(service.Spec.ConfigurationSpec.Template.Spec.PodSpec.Containers[0].Env, propagateEnv)
	}
	return service, nil
}

func constructResourceRequirements(graph v1alpha1api.InferenceGraph, config RouterConfig) v1.ResourceRequirements {
	var specResources v1.ResourceRequirements
	if !reflect.ValueOf(graph.Spec.Resources).IsZero() {
		log.Info("Ignoring defaults for ResourceRequirements as spec has resources mentioned", "specResources", graph.Spec.Resources)
		specResources = v1.ResourceRequirements{
			Limits:   graph.Spec.Resources.Limits,
			Requests: graph.Spec.Resources.Requests,
		}
	} else {
		specResources = v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse(config.CpuLimit),
				v1.ResourceMemory: resource.MustParse(config.MemoryLimit),
			},
			Requests: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse(config.CpuRequest),
				v1.ResourceMemory: resource.MustParse(config.MemoryRequest),
			},
		}
	}
	return specResources
}

// setAutoScalingAnnotations checks the knative autoscaler configuration defined in the knativeserving custom resource
// and compares the values to the autoscaling configuration requested for the inference service.
// It then sets the necessary annotations for the desired autoscaling configuration.
func setAutoScalingAnnotations(client client.Client,
	annotations map[string]string) error {
	// User can pass down scaling class annotation to overwrite the default scaling KPA
	if _, ok := annotations[autoscaling.ClassAnnotationKey]; !ok {
		annotations[autoscaling.ClassAnnotationKey] = autoscaling.KPA
	}

	// If a min-scale annotation is not set for the inference graph, then use the default min-scale value of 1.
	var revisionMinScale int
	if _, ok := annotations[autoscaling.MinScaleAnnotationKey]; !ok {
		annotations[autoscaling.MinScaleAnnotationKey] = fmt.Sprint(constants.DefaultMinReplicas)
		revisionMinScale = constants.DefaultMinReplicas
	} else {
		minScaleInt, err := strconv.Atoi(annotations[autoscaling.MinScaleAnnotationKey])
		if err != nil {
			return errors.Wrapf(err, "fails to convert existing min-scale annotation to an integer")
		}
		revisionMinScale = minScaleInt
	}

	log.Info("inference graph min scale is set using annotations, it is not based on the requested min replicas value.",
		"min-scale", annotations[autoscaling.MinScaleAnnotationKey])

	// Retrieve the allow-zero-initial-scale and initial-scale values from the knative autoscaler configuration.
	allowZeroInitialScale, globalInitialScale, err := knutils.GetAutoscalerConfiguration(client)
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve the knative autoscaler configuration")
	}

	initialScaleInt, err := strconv.Atoi(globalInitialScale)
	if err != nil {
		return errors.Wrapf(err, fmt.Sprintf("fails to convert configured knative serving global initial-scale value to an integer. initial-scale: %s", globalInitialScale))
	}

	// Provide transparency to users while aligning with knative serving's expected behavior, log a warning when the
	// knative autoscaler's gloabal initial-scale value exceeds the requested minScale value for the inference graph.
	if initialScaleInt > revisionMinScale {
		log.Info("knative autoscaler is globally configured with an initial-scale value that is greater than the requested min-scale for the inference graph.",
			"initial-scale", initialScaleInt,
			"min-scale", revisionMinScale)
	}

	// knative will choose the larger of min-scale and initial-scale as the initial target scale for a knative revision.
	// When min-scale is 0, if allow-zero-initial scale is true, set initial-scale to 0 for the created knative revision.
	// This will prevent any pods from being created to initialize a knative revision when an inference graph has minReplicas set to 0.
	// Configuring scaling for knative: https://knative.dev/docs/serving/autoscaling/scale-bounds/#initial-scale
	if revisionMinScale == 0 {
		if allowZeroInitialScale == "true" {
			log.Info("kserve will override the global knative autoscaler configuration for initial-scale on a per revision basis with 0 when an inference graph is requested with min-scale 0",
				"allow-zero-initial-scale", allowZeroInitialScale,
				"initial-scale", globalInitialScale)
			annotations[constants.InitialScaleAnnotationKey] = "0"
		} else {
			log.Info("The current knative autoscaler global configuration does not allow zero initial scale",
				"allow-zero-initial-scale", allowZeroInitialScale,
				"initial-scale", globalInitialScale)
		}
	}

	return nil
}
