/*
Copyright 2026.

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

package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
	"github.com/Al-Pragliola/ai-pipelines/internal/issuehistory"
	"github.com/Al-Pragliola/ai-pipelines/internal/trigger"
)

const defaultSecretKey = "token"

// PipelineReconciler reconciles a Pipeline object.
type PipelineReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	History *issuehistory.Store

	mu      sync.Mutex
	pollers map[types.NamespacedName]context.CancelFunc
}

// +kubebuilder:rbac:groups=ai.aipipelines.io,resources=pipelines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ai.aipipelines.io,resources=pipelines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ai.aipipelines.io,resources=pipelines/finalizers,verbs=update
// +kubebuilder:rbac:groups=ai.aipipelines.io,resources=pipelineruns,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *PipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var pipeline aiv1alpha1.Pipeline
	if err := r.Get(ctx, req.NamespacedName, &pipeline); err != nil {
		if apierrors.IsNotFound(err) {
			r.stopPoller(req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	trig := pipeline.Spec.Trigger

	// Read trigger credentials from Secret
	var secretRef aiv1alpha1.SecretKeyRef
	switch {
	case trig.GitHub != nil:
		secretRef = trig.GitHub.SecretRef
	case trig.Jira != nil:
		secretRef = trig.Jira.SecretRef
	default:
		log.Info("no trigger configured")
		return ctrl.Result{}, nil
	}

	token, err := r.readSecretKey(ctx, pipeline.Namespace, secretRef)
	if err != nil {
		log.Error(err, "failed to read trigger secret")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// For Jira Cloud, also read email if present
	var jiraEmail string
	if trig.Jira != nil {
		jiraEmail, _ = r.readSecretKeyOptional(ctx, pipeline.Namespace, trig.Jira.SecretRef.Name, "email")
	}

	// Start or restart the poller
	r.ensurePoller(req.NamespacedName, &pipeline, token, jiraEmail)

	// Update status
	pipeline.Status.PollerActive = true
	if err := r.Status().Update(ctx, &pipeline); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *PipelineReconciler) readSecretKey(ctx context.Context, namespace string, ref aiv1alpha1.SecretKeyRef) (string, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, &secret); err != nil {
		return "", fmt.Errorf("getting secret %q: %w", ref.Name, err)
	}
	key := ref.Key
	if key == "" {
		key = defaultSecretKey
	}
	val, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %q", key, ref.Name)
	}
	return string(val), nil
}

func (r *PipelineReconciler) readSecretKeyOptional(ctx context.Context, namespace, secretName, key string) (string, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, &secret); err != nil {
		return "", err
	}
	val, ok := secret.Data[key]
	if !ok {
		return "", nil
	}
	return string(val), nil
}

// ensurePoller starts a new poller or restarts if one is already running.
func (r *PipelineReconciler) ensurePoller(key types.NamespacedName, pipeline *aiv1alpha1.Pipeline, token, jiraEmail string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.pollers == nil {
		r.pollers = make(map[types.NamespacedName]context.CancelFunc)
	}

	// Stop existing poller if running
	if cancel, ok := r.pollers[key]; ok {
		cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.pollers[key] = cancel

	spec := pipeline.Spec.DeepCopy()
	namespace := pipeline.Namespace
	pipelineName := pipeline.Name

	go r.runPoller(ctx, key, namespace, pipelineName, spec, token, jiraEmail)
}

func (r *PipelineReconciler) stopPoller(key types.NamespacedName) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cancel, ok := r.pollers[key]; ok {
		cancel()
		delete(r.pollers, key)
	}
}

func (r *PipelineReconciler) runPoller(ctx context.Context, key types.NamespacedName, namespace, pipelineName string, spec *aiv1alpha1.PipelineSpec, token, jiraEmail string) {
	log := logf.Log.WithName("poller").WithValues("pipeline", key)

	var interval time.Duration
	var err error
	switch {
	case spec.Trigger.GitHub != nil:
		interval, err = time.ParseDuration(spec.Trigger.GitHub.PollInterval)
	case spec.Trigger.Jira != nil:
		interval, err = time.ParseDuration(spec.Trigger.Jira.PollInterval)
	}
	if err != nil || interval == 0 {
		interval = 30 * time.Second
	}

	log.Info("starting poller", "interval", interval)

	// Poll immediately, then on interval
	r.poll(ctx, namespace, pipelineName, key, spec, token, jiraEmail)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("poller stopped")
			return
		case <-ticker.C:
			r.poll(ctx, namespace, pipelineName, key, spec, token, jiraEmail)
		}
	}
}

// trigger.Issue is a trigger-agnostic issue representation.
func (r *PipelineReconciler) poll(ctx context.Context, namespace, pipelineName string, key types.NamespacedName, spec *aiv1alpha1.PipelineSpec, token, jiraEmail string) {
	log := logf.Log.WithName("poller").WithValues("pipeline", key)

	var issues []trigger.Issue
	var err error

	switch {
	case spec.Trigger.GitHub != nil:
		issues, err = trigger.FetchGitHubIssues(ctx, spec.Trigger.GitHub, token)
	case spec.Trigger.Jira != nil:
		issues, err = trigger.FetchJiraIssues(ctx, spec.Trigger.Jira, token, jiraEmail)
	}
	if err != nil {
		log.Error(err, "failed to fetch issues")
		return
	}

	// Update lastPollTime
	now := metav1.Now()
	var pipeline aiv1alpha1.Pipeline
	if err := r.Get(ctx, key, &pipeline); err == nil {
		pipeline.Status.LastPollTime = &now
		_ = r.Status().Update(ctx, &pipeline)
	}

	for _, issue := range issues {
		// Layer 1: check live PipelineRun CRs (handles active runs)
		exists, err := r.pipelineRunExists(ctx, namespace, pipelineName, issue.Key)
		if err != nil {
			log.Error(err, "failed to check existing PipelineRun", "issue", issue.Key)
			continue
		}
		if exists {
			continue
		}

		// Layer 2: check history DB (handles deleted CRs for completed issues)
		if r.History != nil {
			completed, err := r.History.IsCompleted(ctx, namespace, pipelineName, issue.Key)
			if err != nil {
				log.Error(err, "failed to check issue history", "issue", issue.Key)
			}
			if completed {
				continue
			}
		}

		log.Info("creating PipelineRun for new issue", "issue", issue.Key, "title", issue.Title)
		if err := r.createPipelineRun(ctx, namespace, pipelineName, key, issue); err != nil {
			log.Error(err, "failed to create PipelineRun", "issue", issue.Key)
		}
	}
}

// --- Dedup and creation ---

func (r *PipelineReconciler) pipelineRunExists(ctx context.Context, namespace, pipelineName, issueKey string) (bool, error) {
	var runs aiv1alpha1.PipelineRunList
	if err := r.List(ctx, &runs, client.InNamespace(namespace), client.MatchingLabels{
		"ai.aipipelines.io/pipeline":  pipelineName,
		"ai.aipipelines.io/issue-key": sanitizeLabelValue(issueKey),
	}); err != nil {
		return false, err
	}
	return len(runs.Items) > 0, nil
}

func (r *PipelineReconciler) createPipelineRun(ctx context.Context, namespace, pipelineName string, pipelineKey types.NamespacedName, issue trigger.Issue) error {
	var pipeline aiv1alpha1.Pipeline
	if err := r.Get(ctx, pipelineKey, &pipeline); err != nil {
		return err
	}

	run := &aiv1alpha1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", pipelineName),
			Namespace:    namespace,
			Labels: map[string]string{
				"ai.aipipelines.io/pipeline":  pipelineName,
				"ai.aipipelines.io/issue-key": sanitizeLabelValue(issue.Key),
			},
		},
		Spec: aiv1alpha1.PipelineRunSpec{
			PipelineRef: pipelineName,
			IssueNumber: issue.Number,
			IssueKey:    issue.Key,
			IssueTitle:  issue.Title,
			IssueBody:   issue.Body,
		},
	}

	if err := controllerutil.SetControllerReference(&pipeline, run, r.Scheme); err != nil {
		return fmt.Errorf("setting owner reference: %w", err)
	}

	return r.Create(ctx, run)
}

// sanitizeLabelValue ensures a string is valid as a K8s label value.
// Label values must be 63 chars or less, alphanumeric + '-', '_', '.'.
func sanitizeLabelValue(s string) string {
	s = strings.ReplaceAll(s, "#", "")
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiv1alpha1.Pipeline{}).
		Named("pipeline").
		Complete(r)
}
