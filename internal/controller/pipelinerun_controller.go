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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"text/template"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
	"github.com/Al-Pragliola/ai-pipelines/internal/issuehistory"
)

const (
	gitImage       = "alpine/git:latest"
	shellImage     = "ubuntu:24.04"
	readerImage    = "alpine:latest"
	workspacePath  = "/workspace"
	promptPath     = "/tmp/prompt"
	pvcStorageSize = "1Gi"
	triageFile     = "/workspace/.triage.json"
)

// PipelineRunReconciler reconciles a PipelineRun object.
type PipelineRunReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Clientset kubernetes.Interface
	History   *issuehistory.Store
}

// +kubebuilder:rbac:groups=ai.aipipelines.io,resources=pipelineruns,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ai.aipipelines.io,resources=pipelineruns/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ai.aipipelines.io,resources=pipelineruns/finalizers,verbs=update
// +kubebuilder:rbac:groups=ai.aipipelines.io,resources=pipelines,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;create;delete
// +kubebuilder:rbac:groups="",resources=pods/exec,verbs=create
// +kubebuilder:rbac:groups="",resources=pods/log,verbs=get

func (r *PipelineRunReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var run aiv1alpha1.PipelineRun
	if err := r.Get(ctx, req.NamespacedName, &run); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Deleting — clean up resources and delete the CR
	if run.Status.Phase == aiv1alpha1.PipelineRunPhaseDeleting {
		return r.reconcileDeleting(ctx, &run)
	}

	// Terminal states — record in history and stop
	if run.Status.Phase == aiv1alpha1.PipelineRunPhaseSucceeded || run.Status.Phase == aiv1alpha1.PipelineRunPhaseFailed || run.Status.Phase == aiv1alpha1.PipelineRunPhaseStopped {
		r.recordCompletion(ctx, &run)
		return ctrl.Result{}, nil
	}

	// Skipped via dashboard — immediately mark as Stopped before any work starts
	if run.Annotations["ai.aipipelines.io/skipped"] == "true" && (run.Status.Phase == "" || run.Status.Phase == aiv1alpha1.PipelineRunPhasePending) {
		log.Info("issue skipped by user", "issue", run.Spec.IssueKey)
		now := metav1.Now()
		run.Status.Phase = aiv1alpha1.PipelineRunPhaseStopped
		run.Status.StartedAt = &now
		run.Status.FinishedAt = &now
		if err := r.Status().Update(ctx, &run); err != nil {
			return ctrl.Result{}, err
		}
		r.recordCompletion(ctx, &run)
		return ctrl.Result{}, nil
	}

	// Get the parent Pipeline CR for step definitions
	var pipeline aiv1alpha1.Pipeline
	if err := r.Get(ctx, types.NamespacedName{Name: run.Spec.PipelineRef, Namespace: run.Namespace}, &pipeline); err != nil {
		log.Error(err, "failed to get parent Pipeline")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Phase: WaitingForInput — check if user provided repo selection
	if run.Status.Phase == aiv1alpha1.PipelineRunPhaseWaitingForInput {
		return r.reconcileWaiting(ctx, &run, &pipeline)
	}

	// Phase: Pending → initialize
	if run.Status.Phase == "" || run.Status.Phase == aiv1alpha1.PipelineRunPhasePending {
		return r.reconcilePending(ctx, &run, &pipeline)
	}

	// Phase: Running → drive step state machine
	return r.reconcileRunning(ctx, &run, &pipeline)
}

func (r *PipelineRunReconciler) reconcilePending(ctx context.Context, run *aiv1alpha1.PipelineRun, pipeline *aiv1alpha1.Pipeline) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Create workspace PVC
	pvcName := run.Name + "-workspace"
	if err := r.ensurePVC(ctx, run, pvcName); err != nil {
		log.Error(err, "failed to create workspace PVC")
		return ctrl.Result{}, err
	}

	// Render branch name
	branch, err := renderTemplate(pipeline.Spec.Steps, run)
	if err != nil {
		log.Error(err, "failed to render branch template")
	}

	// Initialize status
	now := metav1.Now()
	run.Status.Phase = aiv1alpha1.PipelineRunPhaseRunning
	run.Status.PVCName = pvcName
	run.Status.StartedAt = &now
	run.Status.Branch = branch

	// For single-repo pipelines, resolve the repo immediately
	if pipeline.Spec.Repo != nil {
		run.Status.ResolvedRepo = &aiv1alpha1.SelectedRepo{
			Owner:     pipeline.Spec.Repo.Owner,
			Name:      pipeline.Spec.Repo.Name,
			ForkOwner: pipeline.Spec.Repo.ForkOwner,
		}
	}

	if len(pipeline.Spec.Steps) > 0 {
		run.Status.CurrentStep = pipeline.Spec.Steps[0].Name
	}

	// Initialize step statuses
	run.Status.Steps = make([]aiv1alpha1.StepStatus, len(pipeline.Spec.Steps))
	for i, s := range pipeline.Spec.Steps {
		run.Status.Steps[i] = aiv1alpha1.StepStatus{
			Name:  s.Name,
			Type:  s.Type,
			Phase: aiv1alpha1.PipelineRunPhasePending,
		}
	}

	if err := r.Status().Update(ctx, run); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("pipeline run initialized", "pvc", pvcName, "branch", branch)
	return ctrl.Result{Requeue: true}, nil
}

func (r *PipelineRunReconciler) reconcileRunning(ctx context.Context, run *aiv1alpha1.PipelineRun, pipeline *aiv1alpha1.Pipeline) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Find current step config
	stepIdx, stepSpec := r.findStep(pipeline, run.Status.CurrentStep)
	if stepSpec == nil {
		return r.failRun(ctx, run, fmt.Sprintf("step %q not found in pipeline", run.Status.CurrentStep))
	}

	stepStatus := &run.Status.Steps[stepIdx]

	// Check if a Job exists for the current step + attempt
	attempt := stepStatus.Attempt
	if attempt == 0 {
		attempt = 1
	}
	jobName := fmt.Sprintf("%s-%s-%d", run.Name, stepSpec.Name, attempt)

	var job batchv1.Job
	err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: run.Namespace}, &job)

	if apierrors.IsNotFound(err) {
		// Check if step requires approval before creating the job
		if stepSpec.RequireApproval && run.Spec.ApprovedStep != stepSpec.Name {
			if run.Status.Phase != aiv1alpha1.PipelineRunPhaseWaitingForInput {
				log.Info("step requires approval, waiting", "step", stepSpec.Name)

				// Create diff preview Job so the user can review changes
				diffJobName, err := r.createDiffJob(ctx, run)
				if err != nil {
					log.Error(err, "failed to create diff preview job (non-fatal)")
				}

				run.Status.Phase = aiv1alpha1.PipelineRunPhaseWaitingForInput
				run.Status.WaitingFor = "step-approval"
				run.Status.DiffJobName = diffJobName
				stepStatus.Phase = aiv1alpha1.PipelineRunPhaseWaitingForInput
				if err := r.Status().Update(ctx, run); err != nil {
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{}, nil
		}

		// Create the Job
		log.Info("creating job for step", "step", stepSpec.Name, "attempt", attempt)

		job, err := r.buildJob(ctx, run, pipeline, stepSpec, jobName, attempt)
		if err != nil {
			log.Error(err, "failed to build job spec", "step", stepSpec.Name)
			return r.failRun(ctx, run, fmt.Sprintf("failed to build job for step %q: %v", stepSpec.Name, err))
		}

		if err := r.Create(ctx, job); err != nil {
			return ctrl.Result{}, err
		}

		// Update step status
		now := metav1.Now()
		if stepSpec.DinD {
			stepStatus.Phase = aiv1alpha1.PipelineRunPhaseInitializing
		} else {
			stepStatus.Phase = aiv1alpha1.PipelineRunPhaseRunning
		}
		stepStatus.Attempt = attempt
		stepStatus.JobName = jobName
		stepStatus.StartedAt = &now

		if err := r.Status().Update(ctx, run); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}

	// Check if step is initializing (init containers still running)
	if stepStatus.Phase == aiv1alpha1.PipelineRunPhaseInitializing {
		pods, err := r.Clientset.CoreV1().Pods(run.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "job-name=" + jobName,
		})
		if err == nil && len(pods.Items) > 0 {
			pod := pods.Items[0]
			mainRunning := false
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.State.Running != nil {
					mainRunning = true
					break
				}
			}
			if mainRunning {
				stepStatus.Phase = aiv1alpha1.PipelineRunPhaseRunning
				if err := r.Status().Update(ctx, run); err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}

	// Job exists — check its status
	if job.Status.Succeeded > 0 {
		// Step succeeded
		now := metav1.Now()
		stepStatus.Phase = aiv1alpha1.PipelineRunPhaseSucceeded
		stepStatus.FinishedAt = &now
		log.Info("step succeeded", "step", stepSpec.Name)

		// Special handling for triage steps
		if stepSpec.Type == "triage" {
			result, err := r.readTriageResult(ctx, run.Namespace, jobName)
			if err != nil {
				return r.failRun(ctx, run, fmt.Sprintf("failed to read triage result: %v", err))
			}

			run.Status.TriageResult = result

			threshold := parseConfidenceThreshold(stepSpec.ConfidenceThreshold)
			confidence := parseConfidenceThreshold(result.Confidence)
			if confidence >= threshold {
				// Auto-select repo
				parts := strings.SplitN(result.Repo, "/", 2)
				if len(parts) == 2 {
					selected := &aiv1alpha1.SelectedRepo{
						Owner: parts[0],
						Name:  parts[1],
					}
					// Look up fork owner from candidates
					for _, c := range pipeline.Spec.Repos {
						if c.Owner == selected.Owner && c.Name == selected.Name {
							selected.ForkOwner = c.ForkOwner
							break
						}
					}
					run.Status.ResolvedRepo = selected
					log.Info("triage auto-selected repo", "repo", result.Repo, "confidence", confidence)
				} else {
					return r.failRun(ctx, run, fmt.Sprintf("triage returned invalid repo format: %q", result.Repo))
				}
			} else {
				// Confidence too low — wait for user input
				log.Info("triage confidence below threshold, waiting for user input",
					"confidence", confidence, "threshold", threshold)
				run.Status.Phase = aiv1alpha1.PipelineRunPhaseWaitingForInput
				run.Status.WaitingFor = "repo-selection"
				if err := r.Status().Update(ctx, run); err != nil {
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, nil
			}
		}

		// Advance to next step
		if stepIdx+1 >= len(pipeline.Spec.Steps) {
			// All steps done
			run.Status.Phase = aiv1alpha1.PipelineRunPhaseSucceeded
			run.Status.CurrentStep = ""
			run.Status.FinishedAt = &now
			log.Info("pipeline run succeeded")
		} else {
			nextStep := pipeline.Spec.Steps[stepIdx+1]
			run.Status.CurrentStep = nextStep.Name
		}

		if err := r.Status().Update(ctx, run); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if job.Status.Failed > 0 {
		// Step failed
		log.Info("step failed", "step", stepSpec.Name, "attempt", attempt)

		if stepSpec.OnFailure != "" {
			maxRetries := stepSpec.MaxRetries
			if maxRetries == 0 {
				maxRetries = 3
			}
			if attempt >= maxRetries {
				return r.failRun(ctx, run, fmt.Sprintf("step %q failed after %d retries", stepSpec.Name, maxRetries))
			}

			// Jump back to onFailure target step
			targetIdx, targetStep := r.findStep(pipeline, stepSpec.OnFailure)
			if targetStep == nil {
				return r.failRun(ctx, run, fmt.Sprintf("on_failure target %q not found", stepSpec.OnFailure))
			}

			log.Info("retrying from step", "from", stepSpec.Name, "to", stepSpec.OnFailure, "attempt", attempt+1)

			// Mark current step as failed
			now := metav1.Now()
			stepStatus.Phase = aiv1alpha1.PipelineRunPhaseFailed
			stepStatus.FinishedAt = &now

			// Increment attempt on the failing step (the test step), jump back to target
			stepStatus.Attempt = attempt + 1

			// Reset target step for re-run
			run.Status.Steps[targetIdx].Phase = aiv1alpha1.PipelineRunPhasePending
			run.Status.Steps[targetIdx].JobName = ""
			run.Status.Steps[targetIdx].StartedAt = nil
			run.Status.Steps[targetIdx].FinishedAt = nil
			run.Status.Steps[targetIdx].Attempt = attempt + 1

			run.Status.CurrentStep = stepSpec.OnFailure

			if err := r.Status().Update(ctx, run); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}

		return r.failRun(ctx, run, fmt.Sprintf("step %q failed", stepSpec.Name))
	}

	// Job is still running — requeue
	return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

func (r *PipelineRunReconciler) reconcileDeleting(ctx context.Context, run *aiv1alpha1.PipelineRun) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("deleting pipeline run", "name", run.Name)

	// Delete all jobs for this run
	for _, step := range run.Status.Steps {
		if step.JobName != "" {
			propagation := metav1.DeletePropagationBackground
			err := r.Clientset.BatchV1().Jobs(run.Namespace).Delete(ctx, step.JobName, metav1.DeleteOptions{
				PropagationPolicy: &propagation,
			})
			if err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "failed to delete job", "job", step.JobName)
			}
		}
	}

	// Delete diff preview job if it exists
	if run.Status.DiffJobName != "" {
		propagation := metav1.DeletePropagationBackground
		err := r.Clientset.BatchV1().Jobs(run.Namespace).Delete(ctx, run.Status.DiffJobName, metav1.DeleteOptions{
			PropagationPolicy: &propagation,
		})
		if err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "failed to delete diff job", "job", run.Status.DiffJobName)
		}
	}

	// Delete chat pod if it exists
	if run.Status.ChatPodName != "" {
		err := r.Clientset.CoreV1().Pods(run.Namespace).Delete(ctx, run.Status.ChatPodName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "failed to delete chat pod", "pod", run.Status.ChatPodName)
		}
	}

	// Delete the PVC if it exists
	if run.Status.PVCName != "" {
		err := r.Clientset.CoreV1().PersistentVolumeClaims(run.Namespace).Delete(ctx, run.Status.PVCName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			log.Error(err, "failed to delete PVC", "pvc", run.Status.PVCName)
		}
	}

	// Delete the PipelineRun CR
	if err := r.Delete(ctx, run); err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	log.Info("pipeline run deleted", "name", run.Name)
	return ctrl.Result{}, nil
}

func (r *PipelineRunReconciler) reconcileWaiting(ctx context.Context, run *aiv1alpha1.PipelineRun, pipeline *aiv1alpha1.Pipeline) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	switch run.Status.WaitingFor {
	case "step-approval":
		if run.Spec.ApprovedStep != run.Status.CurrentStep {
			// Still waiting — spec change will trigger reconciliation
			return ctrl.Result{}, nil
		}
		log.Info("user approved step", "step", run.Status.CurrentStep)

		// Clean up the diff preview Job and chat Pod before the push Job needs the PVC
		r.deleteDiffJob(ctx, run)
		if run.Status.ChatPodName != "" {
			err := r.Clientset.CoreV1().Pods(run.Namespace).Delete(ctx, run.Status.ChatPodName, metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "failed to delete chat pod", "pod", run.Status.ChatPodName)
			}
		}

		run.Status.Phase = aiv1alpha1.PipelineRunPhaseRunning
		run.Status.WaitingFor = ""
		run.Status.DiffJobName = ""
		run.Status.ChatPodName = ""
		if err := r.Status().Update(ctx, run); err != nil {
			return ctrl.Result{}, err
		}

	default: // "repo-selection"
		if run.Spec.SelectedRepo == nil {
			// Still waiting — spec change will trigger reconciliation
			return ctrl.Result{}, nil
		}
		log.Info("user provided repo selection",
			"repo", run.Spec.SelectedRepo.Owner+"/"+run.Spec.SelectedRepo.Name)

		run.Status.ResolvedRepo = run.Spec.SelectedRepo.DeepCopy()
		// Look up fork owner from candidates
		for _, c := range pipeline.Spec.Repos {
			if c.Owner == run.Status.ResolvedRepo.Owner && c.Name == run.Status.ResolvedRepo.Name {
				run.Status.ResolvedRepo.ForkOwner = c.ForkOwner
				break
			}
		}
		run.Status.Phase = aiv1alpha1.PipelineRunPhaseRunning
		run.Status.WaitingFor = ""

		// Advance past the triage step to the next one
		stepIdx, _ := r.findStep(pipeline, run.Status.CurrentStep)
		if stepIdx >= 0 && stepIdx+1 < len(pipeline.Spec.Steps) {
			run.Status.CurrentStep = pipeline.Spec.Steps[stepIdx+1].Name
		} else {
			now := metav1.Now()
			run.Status.Phase = aiv1alpha1.PipelineRunPhaseSucceeded
			run.Status.FinishedAt = &now
		}

		if err := r.Status().Update(ctx, run); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{Requeue: true}, nil
}

func (r *PipelineRunReconciler) findStep(pipeline *aiv1alpha1.Pipeline, name string) (int, *aiv1alpha1.StepSpec) {
	for i := range pipeline.Spec.Steps {
		if pipeline.Spec.Steps[i].Name == name {
			return i, &pipeline.Spec.Steps[i]
		}
	}
	return -1, nil
}

func (r *PipelineRunReconciler) failRun(ctx context.Context, run *aiv1alpha1.PipelineRun, message string) (ctrl.Result, error) {
	now := metav1.Now()
	run.Status.Phase = aiv1alpha1.PipelineRunPhaseFailed
	run.Status.FinishedAt = &now

	// Update current step status if it exists
	for i := range run.Status.Steps {
		if run.Status.Steps[i].Name == run.Status.CurrentStep {
			run.Status.Steps[i].Phase = aiv1alpha1.PipelineRunPhaseFailed
			run.Status.Steps[i].FinishedAt = &now
			run.Status.Steps[i].Message = message
			break
		}
	}

	if err := r.Status().Update(ctx, run); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *PipelineRunReconciler) recordCompletion(ctx context.Context, run *aiv1alpha1.PipelineRun) {
	if r.History == nil {
		return
	}
	issueKey := run.Spec.IssueKey
	if issueKey == "" {
		issueKey = fmt.Sprintf("#%d", run.Spec.IssueNumber)
	}
	completedAt := time.Now()
	if run.Status.FinishedAt != nil {
		completedAt = run.Status.FinishedAt.Time
	}
	if err := r.History.MarkCompleted(ctx, issuehistory.Record{
		PipelineNamespace: run.Namespace,
		PipelineName:      run.Spec.PipelineRef,
		IssueKey:          issueKey,
		Phase:             string(run.Status.Phase),
		RunName:           run.Name,
		CompletedAt:       completedAt,
	}); err != nil {
		logf.FromContext(ctx).Error(err, "failed to record issue completion",
			"issue", issueKey, "phase", run.Status.Phase)
	}
}

// --- Repo resolution ---

type resolvedRepoInfo struct {
	Owner     string
	Name      string
	ForkOwner string
	SecretRef aiv1alpha1.SecretKeyRef
}

func (r *PipelineRunReconciler) resolveRepoInfo(pipeline *aiv1alpha1.Pipeline, run *aiv1alpha1.PipelineRun) (*resolvedRepoInfo, error) {
	// Single-repo pipeline
	if pipeline.Spec.Repo != nil && run.Status.ResolvedRepo == nil {
		return &resolvedRepoInfo{
			Owner:     pipeline.Spec.Repo.Owner,
			Name:      pipeline.Spec.Repo.Name,
			ForkOwner: pipeline.Spec.Repo.ForkOwner,
			SecretRef: pipeline.Spec.Repo.SecretRef,
		}, nil
	}

	// Triage-resolved or user-selected repo
	if run.Status.ResolvedRepo != nil {
		// Look for matching candidate for ForkOwner and SecretRef
		for _, c := range pipeline.Spec.Repos {
			if c.Owner == run.Status.ResolvedRepo.Owner && c.Name == run.Status.ResolvedRepo.Name {
				info := &resolvedRepoInfo{
					Owner:     c.Owner,
					Name:      c.Name,
					ForkOwner: c.ForkOwner,
				}
				if c.SecretRef != nil {
					info.SecretRef = *c.SecretRef
				} else if pipeline.Spec.Repo != nil {
					info.SecretRef = pipeline.Spec.Repo.SecretRef
				} else {
					return nil, fmt.Errorf("no secretRef for repo %s/%s — set secretRef on the repo candidate", c.Owner, c.Name)
				}
				return info, nil
			}
		}

		// ResolvedRepo not found in candidates — fall back to pipeline.Spec.Repo for SecretRef
		if pipeline.Spec.Repo != nil {
			return &resolvedRepoInfo{
				Owner:     run.Status.ResolvedRepo.Owner,
				Name:      run.Status.ResolvedRepo.Name,
				ForkOwner: pipeline.Spec.Repo.ForkOwner,
				SecretRef: pipeline.Spec.Repo.SecretRef,
			}, nil
		}
		return nil, fmt.Errorf("resolved repo %s/%s not found in candidates and no default secretRef",
			run.Status.ResolvedRepo.Owner, run.Status.ResolvedRepo.Name)
	}

	return nil, fmt.Errorf("no repo configured — set spec.repo or use a triage step with spec.repos")
}

// --- Diff preview ---

func (r *PipelineRunReconciler) createDiffJob(ctx context.Context, run *aiv1alpha1.PipelineRun) (string, error) {
	jobName := run.Name + "-diff-preview"

	// Idempotent: skip if already exists
	var existing batchv1.Job
	if err := r.Get(ctx, types.NamespacedName{Name: jobName, Namespace: run.Namespace}, &existing); err == nil {
		return jobName, nil
	}

	var backoffLimit int32 = 0
	script := fmt.Sprintf(
		`cd %s && git config --global --add safe.directory %s && git add -A && git diff --cached --no-color HEAD -- ':!.test-failures.md'`,
		workspacePath, workspacePath,
	)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: run.Namespace,
			Labels: map[string]string{
				"ai.aipipelines.io/pipeline-run": run.Name,
				"ai.aipipelines.io/diff-preview": "true",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						{
							Name: "workspace",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: run.Status.PVCName,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "diff",
							Image:   gitImage,
							Command: []string{"/bin/sh", "-c"},
							Args:    []string{script},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "workspace", MountPath: workspacePath},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(run, job, r.Scheme); err != nil {
		return "", err
	}
	if err := r.Create(ctx, job); err != nil {
		return "", err
	}
	return jobName, nil
}

func (r *PipelineRunReconciler) deleteDiffJob(ctx context.Context, run *aiv1alpha1.PipelineRun) {
	if run.Status.DiffJobName == "" {
		return
	}
	propagation := metav1.DeletePropagationBackground
	err := r.Clientset.BatchV1().Jobs(run.Namespace).Delete(ctx, run.Status.DiffJobName, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	if err != nil && !apierrors.IsNotFound(err) {
		logf.FromContext(ctx).Error(err, "failed to delete diff job", "job", run.Status.DiffJobName)
	}
}

// --- Resource creation ---

func (r *PipelineRunReconciler) ensurePVC(ctx context.Context, run *aiv1alpha1.PipelineRun, name string) error {
	var existing corev1.PersistentVolumeClaim
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: run.Namespace}, &existing); err == nil {
		return nil // already exists
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: run.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(pvcStorageSize),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(run, pvc, r.Scheme); err != nil {
		return err
	}
	return r.Create(ctx, pvc)
}

func (r *PipelineRunReconciler) buildJob(ctx context.Context, run *aiv1alpha1.PipelineRun, pipeline *aiv1alpha1.Pipeline, step *aiv1alpha1.StepSpec, jobName string, attempt int) (*batchv1.Job, error) {
	var backoffLimit int32 = 0

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: run.Namespace,
			Labels: map[string]string{
				"ai.aipipelines.io/pipeline-run": run.Name,
				"ai.aipipelines.io/step":         step.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						{
							Name: "workspace",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: run.Status.PVCName,
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(run, job, r.Scheme); err != nil {
		return nil, err
	}

	switch step.Type {
	case "git-checkout":
		if err := r.configureCheckoutJob(ctx, job, run, pipeline, step); err != nil {
			return nil, err
		}
	case "ai":
		if err := r.configureAIJob(ctx, job, run, pipeline, step); err != nil {
			return nil, err
		}
	case "shell":
		r.configureShellJob(job, step)
	case "git-push":
		if err := r.configurePushJob(ctx, job, run, pipeline); err != nil {
			return nil, err
		}
	case "triage":
		if err := r.configureTriageJob(ctx, job, run, pipeline, step); err != nil {
			return nil, err
		}
	}

	if step.DinD {
		addDinDSidecar(job)
	}

	return job, nil
}

func (r *PipelineRunReconciler) configureCheckoutJob(_ context.Context, job *batchv1.Job, run *aiv1alpha1.PipelineRun, pipeline *aiv1alpha1.Pipeline, step *aiv1alpha1.StepSpec) error {
	repo, err := r.resolveRepoInfo(pipeline, run)
	if err != nil {
		return fmt.Errorf("resolving repo for checkout: %w", err)
	}

	forkOwner := repo.ForkOwner
	if forkOwner == "" {
		forkOwner = repo.Owner
	}

	branch := run.Status.Branch

	// Clone, configure git, add fork remote, create branch.
	// Credentials are stripped from remote URLs after checkout so AI/shell steps
	// cannot extract the token from .git/config.
	script := fmt.Sprintf(`set -e
rm -rf %s/* %s/.[!.]*
git clone https://x-access-token:${GITHUB_TOKEN}@github.com/%s/%s.git %s
cd %s
git config user.name "AI Pipeline"
git config user.email "ai-pipeline@noreply"
git remote add fork https://x-access-token:${GITHUB_TOKEN}@github.com/%s/%s.git
git checkout -b %s
git remote set-url origin https://github.com/%s/%s.git
git remote set-url fork https://github.com/%s/%s.git
chown -R 1000:1000 %s
echo "checked out branch %s (credentials stripped from remotes, ownership set to 1000)"`,
		workspacePath, workspacePath,
		repo.Owner, repo.Name, workspacePath, workspacePath,
		forkOwner, repo.Name,
		branch,
		repo.Owner, repo.Name,
		forkOwner, repo.Name,
		workspacePath,
		branch)

	job.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:    "checkout",
			Image:   gitImage,
			Command: []string{"/bin/sh", "-c"},
			Args:    []string{script},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: workspacePath},
			},
			Env: []corev1.EnvVar{
				{
					Name: "GITHUB_TOKEN",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: repo.SecretRef.Name,
							},
							Key: secretKey(repo.SecretRef),
						},
					},
				},
			},
		},
	}

	return nil
}

func (r *PipelineRunReconciler) configureAIJob(_ context.Context, job *batchv1.Job, run *aiv1alpha1.PipelineRun, pipeline *aiv1alpha1.Pipeline, step *aiv1alpha1.StepSpec) error {
	// Render the prompt template
	prompt, err := renderPrompt(step.PromptTemplate, run, pipeline.Spec.Repos)
	if err != nil {
		return fmt.Errorf("rendering prompt: %w", err)
	}

	// Build env vars from pipeline AI config
	var envVars []corev1.EnvVar
	for k, v := range pipeline.Spec.AI.Env {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}

	// Volume mounts: workspace + optional credentials
	volumeMounts := []corev1.VolumeMount{
		{Name: "workspace", MountPath: workspacePath},
	}

	if pipeline.Spec.AI.SecretRef != nil {
		mountPath := pipeline.Spec.AI.CredentialsMountPath
		if mountPath == "" {
			mountPath = "/tmp/gcp-creds.json"
		}

		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "ai-credentials",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: pipeline.Spec.AI.SecretRef.Name,
				},
			},
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "ai-credentials",
			MountPath: mountPath,
			SubPath:   secretKey(*pipeline.Spec.AI.SecretRef),
		})
	}

	// Store prompt in a ConfigMap, mount it, and pipe to claude
	cmName := job.Name + "-prompt"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: job.Namespace,
		},
		Data: map[string]string{
			"prompt.txt": prompt,
		},
	}
	if err := controllerutil.SetControllerReference(run, cm, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(context.Background(), cm); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating prompt configmap: %w", err)
	}

	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: "prompt",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
			},
		},
	})

	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "prompt",
		MountPath: promptPath,
	})

	// Override entrypoint to pipe the prompt file to claude
	script := fmt.Sprintf("cat %s/prompt.txt | claude -p --dangerously-skip-permissions --model claude-opus-4-6 --verbose --output-format stream-json",
		promptPath)

	// If failureFile is set, check for it after Claude exits and fail the job accordingly
	if step.FailureFile != "" {
		script += fmt.Sprintf(` ; CLAUDE_EXIT=$?; if [ -f "%s" ]; then echo "---"; echo "STEP FAILED: failure report found at %s"; exit 1; fi; exit $CLAUDE_EXIT`,
			step.FailureFile, step.FailureFile)
	}

	// Security hardening for AI pods:
	// - Run as non-root (Claude CLI refuses root anyway)
	// - Block privilege escalation (prevents su/sudo/setuid)
	// - Drop all capabilities
	var runAsUser int64 = 1000
	var runAsGroup int64 = 1000
	var runAsNonRoot = true
	var allowPrivEsc = false

	job.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
		RunAsUser:    &runAsUser,
		RunAsGroup:   &runAsGroup,
		FSGroup:      &runAsGroup,
		RunAsNonRoot: &runAsNonRoot,
	}

	containerSecurity := &corev1.SecurityContext{
		AllowPrivilegeEscalation: &allowPrivEsc,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}

	// Set HOME so claude can write its config
	envVars = append(envVars, corev1.EnvVar{Name: "HOME", Value: "/tmp"})

	job.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:            "ai",
			Image:           pipeline.Spec.AI.Image,
			ImagePullPolicy: pipeline.Spec.AI.ImagePullPolicy,
			Command:         []string{"/bin/sh", "-c"},
			Args:            []string{script},
			WorkingDir:      workspacePath,
			Env:             envVars,
			VolumeMounts:    volumeMounts,
			SecurityContext: containerSecurity,
		},
	}

	return nil
}

func (r *PipelineRunReconciler) configureShellJob(job *batchv1.Job, step *aiv1alpha1.StepSpec) {
	image := shellImage
	if step.Image != "" {
		image = step.Image
	}

	script := strings.Join(step.Commands, " && ")

	job.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:       "shell",
			Image:      image,
			Command:    []string{"/bin/sh", "-c"},
			Args:       []string{script},
			WorkingDir: workspacePath,
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: workspacePath},
			},
		},
	}
}

// addDinDSidecar injects a Docker-in-Docker sidecar into a Job.
// Uses K8s native sidecars (init container with restartPolicy: Always) so the
// Job completes normally when the main container exits.
// A second init container copies the Docker CLI binary to a shared volume,
// making it available in the main container without requiring a custom image.
func addDinDSidecar(job *batchv1.Job) {
	var rootUser int64 = 0
	var notNonRoot = false
	restartAlways := corev1.ContainerRestartPolicyAlways

	// Volumes for DinD — no hostPath cgroup mount.
	// We mount a fresh cgroup2 filesystem inside the container instead, which
	// respects the container's cgroup namespace and shows its cgroup as root.
	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes,
		corev1.Volume{
			Name:         "dind-storage",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
		corev1.Volume{
			Name:         "docker-bin",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
	)

	// Prepend sidecar + CLI setup as init containers (before any existing ones).
	// 1. DinD sidecar: runs with specific capabilities (not privileged) to avoid
	//    OCI device node errors in nested container environments (Kind on Podman).
	//    Mounts a fresh cgroup2 filesystem over the runtime's read-only mount.
	//    The container's cgroup namespace makes its own cgroup appear as root,
	//    so Docker can manage sub-cgroups naturally without --cgroup-parent hacks.
	// 2. CLI setup: copies docker binary to shared volume, then exits.

	dindInitContainers := []corev1.Container{
		{
			Name:          "dind",
			Image:         "docker:27-dind",
			RestartPolicy: &restartAlways,
			Command:       []string{"/bin/sh", "-c"},
			Args: []string{`set -e
find /run /var/run -name 'docker*.pid' -exec rm -f {} + 2>/dev/null || true
# Unmount the runtime's read-only cgroup bind mount, then mount a fresh
# writable cgroup2. The container's cgroup namespace makes our cgroup appear as root.
umount /sys/fs/cgroup
mount -t cgroup2 cgroup2 /sys/fs/cgroup
# Standard cgroup v2 nesting: move procs to init, enable controller delegation.
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
  mkdir -p /sys/fs/cgroup/init
  for p in $(cat /sys/fs/cgroup/cgroup.procs); do
    echo "$p" > /sys/fs/cgroup/init/cgroup.procs 2>/dev/null || true
  done
  sed -e 's/ / +/g' -e 's/^/+/' < /sys/fs/cgroup/cgroup.controllers \
    > /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null || true
fi
# Mount a fresh writable procfs so Docker can manage sysctls
# (ip_forward, disable_ipv6 on interfaces, etc.)
mount -t proc proc /proc
exec dockerd --host=tcp://0.0.0.0:2375 --host=unix:///var/run/docker.sock --storage-driver=vfs --tls=false --iptables=false`},
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:    &rootUser,
				RunAsNonRoot: &notNonRoot,
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{"SYS_ADMIN", "NET_ADMIN", "MKNOD", "SYS_CHROOT", "AUDIT_WRITE", "SETGID", "SETUID", "CHOWN", "DAC_OVERRIDE", "FOWNER", "KILL", "NET_BIND_SERVICE", "NET_RAW", "SYS_PTRACE"},
				},
			},
			Env: []corev1.EnvVar{
				{Name: "DOCKER_TLS_CERTDIR", Value: ""},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "dind-storage", MountPath: "/var/lib/docker"},
			},
		},
		{
			Name:    "docker-cli-setup",
			Image:   "docker:27-cli",
			Command: []string{"cp", "/usr/local/bin/docker", "/docker-bin/docker"},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "docker-bin", MountPath: "/docker-bin"},
			},
		},
	}
	job.Spec.Template.Spec.InitContainers = append(dindInitContainers, job.Spec.Template.Spec.InitContainers...)

	// Inject Docker env + CLI mount into all main containers
	for i := range job.Spec.Template.Spec.Containers {
		c := &job.Spec.Template.Spec.Containers[i]
		c.Env = append(c.Env, corev1.EnvVar{Name: "DOCKER_HOST", Value: "tcp://localhost:2375"})
		c.VolumeMounts = append(c.VolumeMounts, corev1.VolumeMount{
			Name: "docker-bin", MountPath: "/usr/local/bin/docker", SubPath: "docker", ReadOnly: true,
		})
	}
}

func (r *PipelineRunReconciler) configurePushJob(_ context.Context, job *batchv1.Job, run *aiv1alpha1.PipelineRun, pipeline *aiv1alpha1.Pipeline) error {
	repo, err := r.resolveRepoInfo(pipeline, run)
	if err != nil {
		return fmt.Errorf("resolving repo for push: %w", err)
	}

	forkOwner := repo.ForkOwner
	if forkOwner == "" {
		forkOwner = repo.Owner
	}

	branch := run.Status.Branch

	issueRef := run.Spec.IssueKey
	if issueRef == "" {
		issueRef = fmt.Sprintf("#%d", run.Spec.IssueNumber)
	}

	// Re-inject credentials into the fork remote (stripped during checkout),
	// then apply guardrails and push.
	script := fmt.Sprintf(`set -e
cd %s
git config --global --add safe.directory %s
git remote set-url fork https://x-access-token:${GITHUB_TOKEN}@github.com/%s/%s.git
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$CURRENT_BRANCH" = "main" ] || [ "$CURRENT_BRANCH" = "master" ]; then
  echo "REFUSING TO PUSH: HEAD is on $CURRENT_BRANCH"
  exit 1
fi
if [ "$CURRENT_BRANCH" != "%s" ]; then
  echo "REFUSING TO PUSH: expected branch %s but HEAD is on $CURRENT_BRANCH"
  exit 1
fi
git add .
if git diff --cached --quiet; then
  echo "no changes to commit"
  exit 0
fi
git commit -m 'ai: implement issue %s - %s'
git push -u fork %s
echo "pushed branch %s to fork"`,
		workspacePath, workspacePath,
		forkOwner, repo.Name,
		branch, branch,
		issueRef, run.Spec.IssueTitle,
		branch, branch)

	job.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:       "push",
			Image:      gitImage,
			Command:    []string{"/bin/sh", "-c"},
			Args:       []string{script},
			WorkingDir: workspacePath,
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: workspacePath},
			},
			Env: []corev1.EnvVar{
				{
					Name: "GITHUB_TOKEN",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: repo.SecretRef.Name,
							},
							Key: secretKey(repo.SecretRef),
						},
					},
				},
			},
		},
	}

	return nil
}

func (r *PipelineRunReconciler) configureTriageJob(ctx context.Context, job *batchv1.Job, run *aiv1alpha1.PipelineRun, pipeline *aiv1alpha1.Pipeline, step *aiv1alpha1.StepSpec) error {
	log := logf.FromContext(ctx)

	// Build enriched template data with repo metadata fetched from GitHub
	data := newTemplateData(run, nil)
	for _, c := range pipeline.Spec.Repos {
		candidate := repoCandidateData{
			Owner:       c.Owner,
			Name:        c.Name,
			Description: c.Description,
		}
		// Fetch README and file tree from GitHub (best-effort)
		token, err := r.readSecretToken(ctx, run.Namespace, c.SecretRef)
		if err == nil {
			candidate.Readme, candidate.FileTree = fetchRepoMetadata(ctx, c.Owner, c.Name, token)
			log.Info("fetched repo metadata", "repo", c.Owner+"/"+c.Name,
				"readmeLen", len(candidate.Readme), "treeLen", len(candidate.FileTree))
		} else {
			log.Info("skipping repo metadata fetch (no token)", "repo", c.Owner+"/"+c.Name)
		}
		data.RepoCandidates = append(data.RepoCandidates, candidate)
	}

	prompt, err := renderString(step.PromptTemplate, data)
	if err != nil {
		return fmt.Errorf("rendering triage prompt: %w", err)
	}

	// Build env vars from pipeline AI config
	var envVars []corev1.EnvVar
	for k, v := range pipeline.Spec.AI.Env {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}

	// Volume mounts for the AI init container
	volumeMounts := []corev1.VolumeMount{
		{Name: "workspace", MountPath: workspacePath},
	}

	if pipeline.Spec.AI.SecretRef != nil {
		mountPath := pipeline.Spec.AI.CredentialsMountPath
		if mountPath == "" {
			mountPath = "/tmp/gcp-creds.json"
		}

		job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
			Name: "ai-credentials",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: pipeline.Spec.AI.SecretRef.Name,
				},
			},
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "ai-credentials",
			MountPath: mountPath,
			SubPath:   secretKey(*pipeline.Spec.AI.SecretRef),
		})
	}

	// Create prompt ConfigMap
	cmName := job.Name + "-prompt"
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: job.Namespace,
		},
		Data: map[string]string{
			"prompt.txt": prompt,
		},
	}
	if err := controllerutil.SetControllerReference(run, cm, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(context.Background(), cm); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("creating triage prompt configmap: %w", err)
	}

	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: "prompt",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
			},
		},
	})

	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "prompt",
		MountPath: promptPath,
	})

	aiScript := fmt.Sprintf("cat %s/prompt.txt | claude -p --dangerously-skip-permissions --model claude-opus-4-6 --verbose --output-format stream-json",
		promptPath)

	// Security hardening (same as regular AI steps)
	var runAsUser int64 = 1000
	var runAsGroup int64 = 1000
	var runAsNonRoot = true
	var allowPrivEsc = false

	job.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{
		RunAsUser:    &runAsUser,
		RunAsGroup:   &runAsGroup,
		FSGroup:      &runAsGroup,
		RunAsNonRoot: &runAsNonRoot,
	}

	containerSecurity := &corev1.SecurityContext{
		AllowPrivilegeEscalation: &allowPrivEsc,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}

	envVars = append(envVars, corev1.EnvVar{Name: "HOME", Value: "/tmp"})

	// Init container: run AI to write .triage.json
	job.Spec.Template.Spec.InitContainers = []corev1.Container{
		{
			Name:            "ai",
			Image:           pipeline.Spec.AI.Image,
			ImagePullPolicy: pipeline.Spec.AI.ImagePullPolicy,
			Command:         []string{"/bin/sh", "-c"},
			Args:            []string{aiScript},
			WorkingDir:      workspacePath,
			Env:             envVars,
			VolumeMounts:    volumeMounts,
			SecurityContext: containerSecurity,
		},
	}

	// Main container: output .triage.json so controller can read it from pod logs
	job.Spec.Template.Spec.Containers = []corev1.Container{
		{
			Name:    "reader",
			Image:   readerImage,
			Command: []string{"/bin/sh", "-c"},
			Args:    []string{fmt.Sprintf("cat %s", triageFile)},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: workspacePath},
			},
		},
	}

	return nil
}

// --- Triage result reading ---

func (r *PipelineRunReconciler) readTriageResult(ctx context.Context, namespace, jobName string) (*aiv1alpha1.TriageResult, error) {
	pods, err := r.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil {
		return nil, fmt.Errorf("listing pods for job %s: %w", jobName, err)
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pod found for triage job %s", jobName)
	}

	// Read the "reader" container's logs (contains just the triage JSON)
	logStream, err := r.Clientset.CoreV1().Pods(namespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{
		Container: "reader",
	}).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading triage pod logs: %w", err)
	}
	defer logStream.Close()

	data, err := io.ReadAll(logStream)
	if err != nil {
		return nil, fmt.Errorf("reading triage log stream: %w", err)
	}

	// Parse with intermediate struct since AI writes confidence as a JSON number
	// but the CRD stores it as a string.
	var raw struct {
		Repo       string  `json:"repo"`
		Confidence float64 `json:"confidence"`
		Reasoning  string  `json:"reasoning"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &raw); err != nil {
		return nil, fmt.Errorf("parsing triage result JSON: %w (raw: %s)", err, string(data))
	}

	return &aiv1alpha1.TriageResult{
		Repo:       raw.Repo,
		Confidence: strconv.FormatFloat(raw.Confidence, 'f', -1, 64),
		Reasoning:  raw.Reasoning,
	}, nil
}

func parseConfidenceThreshold(s string) float64 {
	if s == "" {
		return 0.7
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.7
	}
	return v
}

// --- Template rendering ---

type templateData struct {
	IssueNumber    int
	IssueKey       string
	IssueTitle     string
	IssueBody      string
	Branch         string
	Timestamp      string
	RepoCandidates []repoCandidateData
}

type repoCandidateData struct {
	Owner       string
	Name        string
	Description string
	Readme      string
	FileTree    string
}

func newTemplateData(run *aiv1alpha1.PipelineRun, repos []aiv1alpha1.RepoCandidate) templateData {
	data := templateData{
		IssueNumber: run.Spec.IssueNumber,
		IssueKey:    run.Spec.IssueKey,
		IssueTitle:  run.Spec.IssueTitle,
		IssueBody:   run.Spec.IssueBody,
		Branch:      run.Status.Branch,
		Timestamp:   run.CreationTimestamp.Format("20060102T1504"),
	}
	for _, c := range repos {
		data.RepoCandidates = append(data.RepoCandidates, repoCandidateData{
			Owner:       c.Owner,
			Name:        c.Name,
			Description: c.Description,
		})
	}
	return data
}

func renderTemplate(steps []aiv1alpha1.StepSpec, run *aiv1alpha1.PipelineRun) (string, error) {
	for _, s := range steps {
		if s.Type == "git-checkout" && s.BranchTemplate != "" {
			return renderString(s.BranchTemplate, newTemplateData(run, nil))
		}
	}
	return "", nil
}

func renderPrompt(promptTemplate string, run *aiv1alpha1.PipelineRun, repos []aiv1alpha1.RepoCandidate) (string, error) {
	return renderString(promptTemplate, newTemplateData(run, repos))
}

func renderString(tmplStr string, data templateData) (string, error) {
	tmpl, err := template.New("t").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// fetchRepoMetadata fetches README content and top-level directory listing
// from GitHub so the triage AI can make informed repo selection decisions
// without needing direct repo access.
func fetchRepoMetadata(ctx context.Context, owner, name, token string) (readme string, fileTree string) {
	// Fetch README
	readmeURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/readme", owner, name)
	if req, err := http.NewRequestWithContext(ctx, "GET", readmeURL, nil); err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github.raw+json")
		if resp, err := http.DefaultClient.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				data, _ := io.ReadAll(resp.Body)
				readme = string(data)
				if len(readme) > 3000 {
					readme = readme[:3000] + "\n... (truncated)"
				}
			}
		}
	}

	// Fetch top-level directory listing
	treeURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/", owner, name)
	if req, err := http.NewRequestWithContext(ctx, "GET", treeURL, nil); err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		if resp, err := http.DefaultClient.Do(req); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var entries []struct {
					Name string `json:"name"`
					Type string `json:"type"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&entries); err == nil {
					var lines []string
					for _, e := range entries {
						if e.Type == "dir" {
							lines = append(lines, e.Name+"/")
						} else {
							lines = append(lines, e.Name)
						}
					}
					fileTree = strings.Join(lines, "\n")
				}
			}
		}
	}

	return
}

func (r *PipelineRunReconciler) readSecretToken(ctx context.Context, namespace string, ref *aiv1alpha1.SecretKeyRef) (string, error) {
	if ref == nil {
		return "", fmt.Errorf("no secretRef configured")
	}
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, &secret); err != nil {
		return "", err
	}
	key := ref.Key
	if key == "" {
		key = "token"
	}
	val, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %q not found in secret %q", key, ref.Name)
	}
	return string(val), nil
}

func secretKey(ref aiv1alpha1.SecretKeyRef) string {
	if ref.Key != "" {
		return ref.Key
	}
	return "token"
}

// SetupWithManager sets up the controller with the Manager.
func (r *PipelineRunReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aiv1alpha1.PipelineRun{}).
		Owns(&batchv1.Job{}).
		Named("pipelinerun").
		Complete(r)
}
